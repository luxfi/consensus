package bootstrap

import (
    "context"
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/engine/chain/block"
    "github.com/luxfi/consensus/engine/chain/getter"
    "github.com/luxfi/consensus/networking/sender"
    "github.com/luxfi/consensus/engine/core/tracker"
    "github.com/luxfi/trace"
)

// Config configures the bootstrapper
type Config struct {
    AllGetsServer       getter.Handler
    Ctx                 *interfaces.Runtime
    Beacons             interface{}
    SampleK             int
    StartupTracker      tracker.Startup
    Sender              sender.Sender
    BootstrapTracker    interface{}
    Timer               interface{}
    AncestorsMaxContainersReceived int
    Blocked             interface{}
    VM                  block.ChainVM
}

// Bootstrapper performs bootstrap operations
type Bootstrapper interface {
    // Start bootstrapping
    Start(ctx context.Context) error
    // Stop bootstrapping
    Stop() error
}

// New creates a new bootstrapper
func New(cfg Config, onFinished func(ctx context.Context, lastReqID uint32) error) (Bootstrapper, error) {
    return &noOpBootstrapper{}, nil
}

// Trace wraps a bootstrapper with tracing
func Trace(bootstrapper Bootstrapper, tracer trace.Tracer) Bootstrapper {
    return bootstrapper
}

type noOpBootstrapper struct{}

func (n *noOpBootstrapper) Start(ctx context.Context) error {
    return nil
}

func (n *noOpBootstrapper) Stop() error {
    return nil
}
