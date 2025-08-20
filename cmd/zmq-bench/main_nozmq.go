// +build !zmq

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintf(os.Stderr, "zmq-bench requires ZMQ support. Build with: go build -tags zmq\n")
	fmt.Fprintf(os.Stderr, "\nTo install ZMQ:\n")
	fmt.Fprintf(os.Stderr, "  macOS:  brew install zeromq\n")
	fmt.Fprintf(os.Stderr, "  Ubuntu: apt-get install libzmq3-dev\n")
	fmt.Fprintf(os.Stderr, "  Alpine: apk add zeromq-dev\n")
	os.Exit(1)
}