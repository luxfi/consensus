package common

import "errors"

// AppError represents an application-level error
type AppError struct {
    error
}

// ErrUndefined is returned when an undefined error occurs
var ErrUndefined = errors.New("undefined error")
