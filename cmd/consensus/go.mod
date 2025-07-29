module github.com/luxfi/consensus/cmd/consensus

go 1.24.5

require (
	github.com/luxfi/consensus v0.0.0
	github.com/luxfi/ids v0.1.0
	github.com/luxfi/zmq4 v1.0.1
	github.com/pebbe/zmq4 v1.4.0
	github.com/spf13/cobra v1.8.0
)

replace (
	github.com/luxfi/consensus => ../../
	github.com/luxfi/ids => ../../../ids
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-zeromq/goczmq/v4 v4.2.2 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/luxfi/crypto v1.0.2 // indirect
	github.com/luxfi/log v0.1.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/exp v0.0.0-20250718183923-645b1fa84792 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)
