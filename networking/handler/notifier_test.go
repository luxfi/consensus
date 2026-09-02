// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handler

import (
	"context"
	"testing"
	"time"
)

type notifierFunc func(context.Context, VMMessage) error

func (f notifierFunc) Notify(ctx context.Context, m VMMessage) error { return f(ctx, m) }

// The forwarding loop runs on a goroutine nobody can recover from, so it must
// not depend on the caller having passed a logger. Log is exported and the
// constructor takes one, which is two ways to leave it unset; Start answers
// both, since it is the one place the loop begins.
//
// Removing the default in Start makes these crash, not fail: the first
// statement of forwardNotification logs, before it does anything else.
func TestNotificationForwarderRunsWithoutALogger(t *testing.T) {
	for name, build := range map[string]func(Notifier, Subscription) *NotificationForwarder{
		"constructed with a nil logger": func(e Notifier, s Subscription) *NotificationForwarder {
			return NewNotificationForwarder(e, s, nil)
		},
		"literal leaving Log unset": func(e Notifier, s Subscription) *NotificationForwarder {
			return &NotificationForwarder{Engine: e, Subscribe: s}
		},
	} {
		t.Run(name, func(t *testing.T) {
			subscribed := make(chan struct{}, 1)
			subscribe := func(ctx context.Context) (VMMessage, error) {
				select {
				case subscribed <- struct{}{}:
				default:
				}
				<-ctx.Done() // park until Stop, so the loop does not spin
				return VMMessage{}, ctx.Err()
			}
			notify := notifierFunc(func(context.Context, VMMessage) error { return nil })

			nf := build(notify, subscribe)
			nf.Start()
			defer nf.Stop()

			select {
			case <-subscribed:
			case <-time.After(5 * time.Second):
				t.Fatal("forwarding loop never reached the subscription")
			}
		})
	}
}
