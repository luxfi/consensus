//go:build ignore

// Copyright (C) 2019-2025, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package simplex

import (
	simplex "github.com/luxfi/bft"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/luxfi/log"
)

// loggerWrapper wraps luxfi/log.Logger to implement simplex.Logger
type loggerWrapper struct {
	log log.Logger
}

// NewLoggerWrapper creates a new logger wrapper
func NewLoggerWrapper(log log.Logger) simplex.Logger {
	return &loggerWrapper{log: log}
}

// Fatal logs a fatal error using zap.Field interface
func (l *loggerWrapper) Fatal(msg string, fields ...zap.Field) {
	logFields := convertToLogFields(fields)
	l.log.Fatal(msg, logFields...)
}

// Error logs an error using zap.Field interface
func (l *loggerWrapper) Error(msg string, fields ...zap.Field) {
	args := convertFieldsToArgs(fields)
	l.log.Error(msg, args...)
}

// Warn logs a warning using zap.Field interface
func (l *loggerWrapper) Warn(msg string, fields ...zap.Field) {
	args := convertFieldsToArgs(fields)
	l.log.Warn(msg, args...)
}

// Info logs an info message using zap.Field interface
func (l *loggerWrapper) Info(msg string, fields ...zap.Field) {
	args := convertFieldsToArgs(fields)
	l.log.Info(msg, args...)
}

// Trace logs a trace message using zap.Field interface
func (l *loggerWrapper) Trace(msg string, fields ...zap.Field) {
	args := convertFieldsToArgs(fields)
	l.log.Trace(msg, args...)
}

// Debug logs a debug message using zap.Field interface
func (l *loggerWrapper) Debug(msg string, fields ...zap.Field) {
	args := convertFieldsToArgs(fields)
	l.log.Debug(msg, args...)
}

// Verbo logs a verbose message using zap.Field interface
func (l *loggerWrapper) Verbo(msg string, fields ...zap.Field) {
	logFields := convertToLogFields(fields)
	l.log.Verbo(msg, logFields...)
}

// convertToLogFields converts zap.Field to log.Field
func convertToLogFields(fields []zap.Field) []log.Field {
	logFields := make([]log.Field, 0, len(fields))
	for _, field := range fields {
		logFields = append(logFields, log.String(field.Key, fieldToString(field)))
	}
	return logFields
}

// convertFieldsToArgs converts zap.Field to key-value pairs for luxfi logger
func convertFieldsToArgs(fields []zap.Field) []interface{} {
	args := make([]interface{}, 0, len(fields)*2)
	for _, field := range fields {
		args = append(args, field.Key, fieldValue(field))
	}
	return args
}

// fieldToString converts a field value to string
func fieldToString(field zap.Field) string {
	switch field.Type {
	case zapcore.StringType:
		return field.String
	case zapcore.ErrorType:
		if field.Interface != nil {
			return field.Interface.(error).Error()
		}
		return "nil"
	default:
		return "<value>"
	}
}

// fieldValue extracts the value from a zap.Field
func fieldValue(field zap.Field) interface{} {
	switch field.Type {
	case zapcore.StringType:
		return field.String
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return field.Integer
	case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
		return uint64(field.Integer)
	case zapcore.Float64Type, zapcore.Float32Type:
		return field.Interface
	case zapcore.BoolType:
		return field.Integer == 1
	case zapcore.ErrorType:
		if field.Interface != nil {
			return field.Interface.(error).Error()
		}
		return "nil"
	case zapcore.StringerType:
		if field.Interface != nil {
			return "<stringer>"
		}
		return "nil"
	default:
		return field.Interface
	}
}

var _ simplex.Logger = (*loggerWrapper)(nil)
