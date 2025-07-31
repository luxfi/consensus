// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Response represents an API response
type Response struct {
	Success bool        `json:"success"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Error represents an API error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewError creates a new API error
func NewError(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, status int, err error) error {
	return WriteJSON(w, status, Response{
		Success: false,
		Error: &Error{
			Code:    status,
			Message: err.Error(),
		},
	})
}

// WriteSuccess writes a success response
func WriteSuccess(w http.ResponseWriter, result interface{}) error {
	return WriteJSON(w, http.StatusOK, Response{
		Success: true,
		Result:  result,
	})
}

// ErrNotFound is returned when a resource is not found
var ErrNotFound = errors.New("not found")

// ErrBadRequest is returned when a request is invalid
var ErrBadRequest = errors.New("bad request")

// ErrInternalServerError is returned when an internal error occurs
var ErrInternalServerError = errors.New("internal server error")

// ErrUnauthorized is returned when a request is unauthorized
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden is returned when a request is forbidden
var ErrForbidden = errors.New("forbidden")

// HTTPError is an error with an HTTP status code
type HTTPError struct {
	Status  int
	Message string
}

// Error returns the error message
func (e HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// NewHTTPError creates a new HTTP error
func NewHTTPError(status int, message string) HTTPError {
	return HTTPError{
		Status:  status,
		Message: message,
	}
}