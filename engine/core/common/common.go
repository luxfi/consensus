// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import "fmt"

// AppError is a structured error type for application-level errors
type AppError struct {
	Code    int32
	Message string
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("app error %d: %s", e.Code, e.Message)
}
