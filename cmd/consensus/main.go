// Command consensus reports the consensus parameters a build ships for a
// named network, as a table or as JSON. Operators use it to see what a
// binary will actually run with, and to diff that between builds.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/luxfi/consensus/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("consensus", flag.ContinueOnError)
	fs.SetOutput(errOut)
	network := fs.String("network", "mainnet", "network to report: mainnet, testnet, local")
	asJSON := fs.Bool("json", false, "report as JSON instead of a table")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	p, ok := paramsFor(*network)
	if !ok {
		fmt.Fprintf(errOut, "unknown network %q: want mainnet, testnet or local\n", *network)
		return 1
	}

	write := writeTable
	if *asJSON {
		write = writeJSON
	}
	if err := write(out, fields(p)); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

// paramsFor reports whether the network is one this build knows. An unknown
// name is refused rather than defaulted, so a typo cannot quietly print some
// other network's parameters while the operator reads them as the one asked for.
//
// config.XChainParams is deliberately absent: it returns a value identical to
// LocalParams, so offering it as a fourth name would answer the same question
// twice and no test could tell the two apart.
func paramsFor(network string) (config.Parameters, bool) {
	switch network {
	case "mainnet":
		return config.MainnetParams(), true
	case "testnet":
		return config.TestnetParams(), true
	case "local":
		return config.LocalParams(), true
	}
	return config.Parameters{}, false
}

type field struct {
	name  string
	value any
}

// fields is the single ordered description of a report, so the table and the
// JSON cannot drift apart in content. Durations and PQMode render through
// String so both formats carry the same human-readable value.
func fields(p config.Parameters) []field {
	return []field{
		{"k", p.K},
		{"alpha", p.Alpha},
		{"beta", p.Beta},
		{"alphaPreference", p.AlphaPreference},
		{"alphaConfidence", p.AlphaConfidence},
		{"betaVirtuous", p.BetaVirtuous},
		{"betaRogue", p.BetaRogue},
		{"concurrentPolls", p.ConcurrentPolls},
		{"concurrentRepolls", p.ConcurrentRepolls},
		{"optimalProcessing", p.OptimalProcessing},
		{"maxOutstandingItems", p.MaxOutstandingItems},
		{"maxItemProcessingTime", p.MaxItemProcessingTime.String()},
		{"parents", p.Parents},
		{"batchSize", p.BatchSize},
		{"blockTime", p.BlockTime.String()},
		{"roundTimeout", p.RoundTO.String()},
		{"convergenceSettleWindow", p.ConvergenceSettleWindow.String()},
		{"gasLimit", p.GasLimit},
		{"pqMode", p.PQMode.String()},
	}
}

func writeTable(w io.Writer, fs []field) error {
	for _, f := range fs {
		if _, err := fmt.Fprintf(w, "%-24s %v\n", f.name, f.value); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w io.Writer, fs []field) error {
	m := make(map[string]any, len(fs))
	for _, f := range fs {
		m[f.name] = f.value
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(m)
}
