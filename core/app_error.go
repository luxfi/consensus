// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

// AppError represents an application error
type AppError struct {
	Code    int
	Message string
}