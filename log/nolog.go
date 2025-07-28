// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package log

import (
	"context"
	"log/slog"

	"github.com/luxfi/log"
	"go.uber.org/zap"
)

// NoLog is a no-op logger implementation that implements the luxfi/log.Logger interface
type NoLog struct{}

// NewNoOpLogger returns a new no-op logger
func NewNoOpLogger() log.Logger {
	return &NoLog{}
}

// Geth-style methods

// With adds context fields (variadic key-value pairs)
func (n NoLog) With(ctx ...interface{}) log.Logger {
	return n
}

// New is an alias for With
func (n NoLog) New(ctx ...interface{}) log.Logger {
	return n
}

// Log logs at the specified level
func (NoLog) Log(level slog.Level, msg string, ctx ...interface{}) {}

// Trace logs at trace level
func (NoLog) Trace(msg string, ctx ...interface{}) {}

// Debug logs at debug level
func (NoLog) Debug(msg string, ctx ...interface{}) {}

// Info logs at info level
func (NoLog) Info(msg string, ctx ...interface{}) {}

// Warn logs at warn level
func (NoLog) Warn(msg string, ctx ...interface{}) {}

// Error logs at error level
func (NoLog) Error(msg string, ctx ...interface{}) {}

// Crit logs at critical level
func (NoLog) Crit(msg string, ctx ...interface{}) {}

// WriteLog logs a message at the specified level
func (NoLog) WriteLog(level slog.Level, msg string, attrs ...any) {}

// Enabled checks if a level is enabled
func (NoLog) Enabled(ctx context.Context, level slog.Level) bool {
	return false
}

// Handler returns the slog handler
func (NoLog) Handler() slog.Handler {
	return nil
}

// Node compatibility methods

// Fatal logs at fatal level
func (NoLog) Fatal(msg string, fields ...zap.Field) {}

// Verbo logs at verbose level
func (NoLog) Verbo(msg string, fields ...zap.Field) {}

// WithFields adds structured context
func (n NoLog) WithFields(fields ...zap.Field) log.Logger {
	return n
}

// WithOptions adds options
func (n NoLog) WithOptions(opts ...zap.Option) log.Logger {
	return n
}

// Additional methods

// SetLevel sets the logging level
func (NoLog) SetLevel(level slog.Level) {}

// GetLevel returns the current logging level
func (NoLog) GetLevel() slog.Level {
	return slog.Level(0)
}

// EnabledLevel checks if a level is enabled
func (NoLog) EnabledLevel(lvl slog.Level) bool {
	return false
}

// StopOnPanic stops on panic
func (NoLog) StopOnPanic() {}

// RecoverAndPanic recovers and panics
func (NoLog) RecoverAndPanic(f func()) {
	f()
}

// RecoverAndExit recovers and exits
func (NoLog) RecoverAndExit(f, exit func()) {
	f()
}

// Stop stops the logger
func (NoLog) Stop() {}

// Write implements io.Writer
func (NoLog) Write(p []byte) (n int, err error) {
	return len(p), nil
}