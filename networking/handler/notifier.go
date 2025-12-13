// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handler

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/core"
	"github.com/luxfi/log"
)

// Re-export core types for convenience
type (
	MessageType = core.MessageType
	VMMessage   = core.Message
)

// Message type constants
const (
	PendingTxs    = core.PendingTxs
	StateSyncDone = core.StateSyncDone
)

// Notifier is the interface for receiving VM notifications
type Notifier interface {
	// Notify is called when the VM has a message for the consensus engine
	Notify(context.Context, VMMessage) error
}

// Subscription is a function that waits for VM events
type Subscription func(context.Context) (VMMessage, error)

// NotificationForwarder forwards VM notifications to the consensus engine
type NotificationForwarder struct {
	Engine    Notifier
	Subscribe Subscription
	Log       log.Logger

	lock      sync.Mutex
	execCtx   context.Context
	cancel    context.CancelFunc
	executing sync.WaitGroup
	started   bool
}

// NewNotificationForwarder creates a new NotificationForwarder
func NewNotificationForwarder(engine Notifier, subscribe Subscription, logger log.Logger) *NotificationForwarder {
	return &NotificationForwarder{
		Engine:    engine,
		Subscribe: subscribe,
		Log:       logger,
	}
}

// Start begins the notification forwarding loop
func (nf *NotificationForwarder) Start() {
	nf.lock.Lock()
	defer nf.lock.Unlock()

	if nf.started {
		return
	}

	nf.started = true
	nf.execCtx, nf.cancel = context.WithCancel(context.Background())
	nf.executing.Add(1)

	go nf.run()
}

// Stop stops the notification forwarding loop
func (nf *NotificationForwarder) Stop() {
	nf.lock.Lock()
	if !nf.started {
		nf.lock.Unlock()
		return
	}
	nf.started = false
	if nf.cancel != nil {
		nf.cancel()
	}
	nf.lock.Unlock()

	nf.executing.Wait()
}

// CheckForEvent triggers a new subscription if the forwarder is waiting
// This should be called when the VM state changes (e.g., after SetPreference, BuildBlock)
func (nf *NotificationForwarder) CheckForEvent() {
	nf.lock.Lock()
	defer nf.lock.Unlock()

	// Cancel current subscription to trigger a new one
	if nf.cancel != nil {
		nf.cancel()
		// Create new context for next subscription
		nf.execCtx, nf.cancel = context.WithCancel(context.Background())
	}
}

// run is the main notification forwarding loop
func (nf *NotificationForwarder) run() {
	defer nf.executing.Done()

	for {
		nf.lock.Lock()
		if nf.execCtx.Err() != nil {
			nf.lock.Unlock()
			return
		}
		ctx := nf.execCtx
		nf.lock.Unlock()

		nf.forwardNotification(ctx)
	}
}

// forwardNotification subscribes to VM events and forwards them to the engine
func (nf *NotificationForwarder) forwardNotification(ctx context.Context) {
	nf.Log.Debug("subscribing to VM notifications")

	// Subscribe to VM events (this will block until an event occurs)
	msg, err := nf.Subscribe(ctx)
	if err != nil {
		if ctx.Err() != nil {
			// Context was cancelled, this is expected during CheckForEvent or Stop
			nf.Log.Debug("subscription cancelled")
			return
		}
		nf.Log.Debug("failed subscribing to VM notifications",
			log.Err(err))
		// Throttle retries on error
		time.Sleep(100 * time.Millisecond)
		return
	}

	nf.Log.Debug("received VM notification",
		log.Uint32("type", uint32(msg.Type)))

	// Forward the notification to the engine
	if err := nf.Engine.Notify(ctx, msg); err != nil {
		nf.Log.Debug("failed notifying engine",
			log.Err(err))
		return
	}

	// Wait for context to be cancelled (by CheckForEvent) before re-subscribing
	// This ensures we don't spam the VM with subscriptions
	<-ctx.Done()
}
