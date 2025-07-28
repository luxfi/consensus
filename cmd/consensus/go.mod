module github.com/luxfi/consensus/cmd/consensus

go 1.21

require (
	github.com/luxfi/consensus v0.0.0
	github.com/luxfi/ids v0.0.0
	github.com/spf13/cobra v1.8.0
	github.com/pebbe/zmq4 v1.2.10
)

replace (
	github.com/luxfi/consensus => ../../
	github.com/luxfi/ids => ../../../ids
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)