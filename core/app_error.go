// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package core

import "fmt"

// AppError represents an application error
type AppError struct {
	Code    int32
	Message string
}

// Error implements the error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("app error %d: %s", e.Code, e.Message)
}
