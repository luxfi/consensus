// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package consensustest provides testing utilities for consensus
package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Fail fails the test with the given error
func Fail(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Assert asserts that a condition is true
func Assert(t testing.TB, condition bool, msg string, args ...interface{}) {
	t.Helper()
	if !condition {
		t.Fatalf(msg, args...)
	}
}

// NoError asserts that an error is nil
func NoError(t testing.TB, err error) {
	t.Helper()
	require.NoError(t, err)
}

// Error asserts that an error is not nil
func Error(t testing.TB, err error) {
	t.Helper()
	require.Error(t, err)
}

// Equal asserts that two values are equal
func Equal(t testing.TB, expected, actual interface{}) {
	t.Helper()
	require.Equal(t, expected, actual)
}

// NotEqual asserts that two values are not equal
func NotEqual(t testing.TB, expected, actual interface{}) {
	t.Helper()
	require.NotEqual(t, expected, actual)
}

// Contains asserts that a string contains a substring
func Contains(t testing.TB, s, contains string) {
	t.Helper()
	require.Contains(t, s, contains)
}

// NotContains asserts that a string does not contain a substring
func NotContains(t testing.TB, s, contains string) {
	t.Helper()
	require.NotContains(t, s, contains)
}

// Nil asserts that a value is nil
func Nil(t testing.TB, v interface{}) {
	t.Helper()
	require.Nil(t, v)
}

// NotNil asserts that a value is not nil
func NotNil(t testing.TB, v interface{}) {
	t.Helper()
	require.NotNil(t, v)
}

// True asserts that a value is true
func True(t testing.TB, v bool) {
	t.Helper()
	require.True(t, v)
}

// False asserts that a value is false
func False(t testing.TB, v bool) {
	t.Helper()
	require.False(t, v)
}
