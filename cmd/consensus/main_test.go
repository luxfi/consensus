package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/luxfi/consensus/config"
)

// An unknown network must be refused, not silently answered with some other
// network's numbers. Breaks if paramsFor grows a default arm.
func TestUnknownNetworkIsRefused(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-network", "mianet"}, &out, &errOut); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty: a refused network must report no parameters", out.String())
	}
	if !strings.Contains(errOut.String(), "mianet") {
		t.Errorf("stderr = %q, want it to name the rejected network", errOut.String())
	}
}

// Each name must reach its own constructor. Breaks if a network is wired to
// the wrong one, because the three differ in K, Beta and BlockTime.
func TestNetworkReachesItsOwnParameters(t *testing.T) {
	for name, want := range map[string]config.Parameters{
		"mainnet": config.MainnetParams(),
		"testnet": config.TestnetParams(),
		"local":   config.LocalParams(),
	} {
		t.Run(name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"-network", name, "-json"}, &out, &errOut); code != 0 {
				t.Fatalf("exit = %d (stderr %q), want 0", code, errOut.String())
			}
			got := decode(t, out.Bytes())
			for _, f := range fields(want) {
				if got[f.name] != fmt.Sprint(f.value) {
					t.Errorf("%s = %q, want %q", f.name, got[f.name], fmt.Sprint(f.value))
				}
			}
		})
	}
}

// The two formats are rendered by different code from one description, so this
// pins that they cannot drift: same names, same values, nothing dropped.
func TestTableAndJSONAgree(t *testing.T) {
	var tbl, js, errOut bytes.Buffer
	if code := run([]string{"-network", "mainnet"}, &tbl, &errOut); code != 0 {
		t.Fatalf("table exit = %d", code)
	}
	if code := run([]string{"-network", "mainnet", "-json"}, &js, &errOut); code != 0 {
		t.Fatalf("json exit = %d", code)
	}

	fromJSON := decode(t, js.Bytes())
	lines := strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")
	if len(lines) != len(fromJSON) {
		t.Fatalf("table has %d rows, JSON has %d keys", len(lines), len(fromJSON))
	}
	for _, line := range lines {
		name, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			t.Fatalf("row %q has no value", line)
		}
		if got, want := strings.TrimSpace(value), fromJSON[name]; got != want {
			t.Errorf("%s: table %q, JSON %q", name, got, want)
		}
	}
}

// The exit code is the tool's contract with a shell. Breaks if -h starts
// reporting failure, or a bad flag starts reporting success.
func TestExitCodes(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want int
	}{
		{"default is mainnet", nil, 0},
		{"help", []string{"-h"}, 0},
		{"unknown network", []string{"-network", "nope"}, 1},
		{"unknown flag", []string{"-verbose"}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(tt.args, &out, &errOut); code != tt.want {
				t.Errorf("exit = %d, want %d", code, tt.want)
			}
		})
	}
}

// A report that could not be written is a failure, not a silent success —
// otherwise `consensus > /full/disk` exits 0 having produced nothing.
func TestWriteFailureIsReported(t *testing.T) {
	for _, args := range [][]string{{}, {"-json"}} {
		var errOut bytes.Buffer
		if code := run(args, brokenWriter{}, &errOut); code != 1 {
			t.Errorf("run(%v) exit = %d, want 1", args, code)
		}
		if !strings.Contains(errOut.String(), "disk full") {
			t.Errorf("run(%v) stderr = %q, want the write error", args, errOut.String())
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// main is what a shell actually runs, so it is exercised as a process: the
// test binary re-executes itself and reads the code it exits with. Breaks if
// os.Exit is dropped (a refused network would then look successful to a
// script) or if os.Args reaches run unsliced (the program name would parse
// as a flag and every invocation would fail).
func TestMainExitsWithRunsCode(t *testing.T) {
	if args, child := os.LookupEnv(childArgs); child {
		os.Args = append([]string{"consensus"}, strings.Fields(args)...)
		main()
		return
	}

	for _, tt := range []struct {
		name string
		args string
		want int
	}{
		{"known network", "-network local", 0},
		{"unknown network", "-network nope", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestMainExitsWithRunsCode")
			cmd.Env = append(os.Environ(), childArgs+"="+tt.args)
			out, err := cmd.Output()

			got := 0
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				got = exit.ExitCode()
			} else if err != nil {
				t.Fatalf("running child: %v", err)
			}
			if got != tt.want {
				t.Fatalf("exit = %d, want %d (stdout %q)", got, tt.want, out)
			}
			if wantReport := tt.want == 0; wantReport != strings.Contains(string(out), "k ") {
				t.Errorf("stdout = %q, report on stdout should be %v", out, wantReport)
			}
		})
	}
}

// childArgs carries the command line into the re-executed test binary.
const childArgs = "CONSENSUS_TEST_ARGS"

// decode renders the JSON report as name -> literal text, so values compare
// against fmt.Sprint of the source field without float round-tripping.
func decode(t *testing.T, b []byte) map[string]string {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var raw map[string]any
	if err := d.Decode(&raw); err != nil {
		t.Fatalf("report is not JSON: %v", err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[k] = fmt.Sprint(v)
	}
	return out
}
