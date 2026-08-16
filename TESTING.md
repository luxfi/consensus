# Testing

Always `GOWORK=off` in this repo.

## The suite

```bash
GOWORK=off go test ./...                    # everything
GOWORK=off go test -race ./...              # what CI runs
GOWORK=off go test -run TestName ./config   # one test
```

`protocol/quasar` is the long pole: ~11600 SLH-DSA tests, which the race
detector multiplies by roughly 22. Give it `-timeout=40m` under `-race`, or it
gets killed mid-run and reports FAIL at exactly the timeout.

## Coverage

```bash
GOWORK=off go test -coverprofile=coverage.out ./...
GOWORK=off go tool cover -func=coverage.out    # per function
GOWORK=off go tool cover -html=coverage.out    # annotated source
```

`make coverage-95` fails the build under 95%.

A coverage number only means something if the tests can fail. Before trusting
one, break the code on purpose and confirm the test that covers it goes red —
a test that passes against a mutilated implementation is measuring nothing.

## Integration

`test/integration` drives the engines through the public API. Nothing there
needs a server or a fixture process; `go test ./test/integration` is the whole
story.

## Benchmarks

```bash
GOWORK=off go test -bench=. -benchmem ./...
make benchmark                              # config, protocol, engine, core
make benchmark-zmq                          # ZAP transport, needs ginkgo
```

Increase `-benchtime=30s` when results scatter. Numbers in `BENCHMARKS.md` were
taken on an M1 Max; treat anything measured elsewhere as a different data set,
not a regression.

## Profiling

```bash
GOWORK=off go test -cpuprofile=cpu.prof -bench=.
GOWORK=off go tool pprof cpu.prof

GOWORK=off go test -memprofile=mem.prof -bench=.
GOWORK=off go tool pprof mem.prof
```

## The CLI

`make build` produces `bin/consensus`, which reports the parameters this build
ships for a network:

```bash
./bin/consensus                       # mainnet, as a table
./bin/consensus -network local -json  # local, as JSON
```

It refuses a network it does not know rather than defaulting, so a typo exits
non-zero instead of printing mainnet's numbers under the name you asked for.

## CI

Native runners via `.hanzo/workflows` — `ci.yml` for the suite, lint and the C
and Rust wrappers, `release.yml` for the cross-compiled `consensus` binaries.

```bash
gh run list --repo luxfi/consensus --limit 5
gh run view <run-id> --repo luxfi/consensus
```
