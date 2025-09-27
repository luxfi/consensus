package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppError(t *testing.T) {
	tests := []struct {
		name     string
		code     int32
		message  string
		expected string
	}{
		{
			name:     "standard error",
			code:     404,
			message:  "not found",
			expected: "app error 404: not found",
		},
		{
			name:     "zero code",
			code:     0,
			message:  "success",
			expected: "app error 0: success",
		},
		{
			name:     "negative code",
			code:     -1,
			message:  "invalid",
			expected: "app error -1: invalid",
		},
		{
			name:     "empty message",
			code:     500,
			message:  "",
			expected: "app error 500: ",
		},
		{
			name:     "large code",
			code:     2147483647,
			message:  "max int32",
			expected: "app error 2147483647: max int32",
		},
		{
			name:     "min int32 code",
			code:     -2147483648,
			message:  "min int32",
			expected: "app error -2147483648: min int32",
		},
		{
			name:     "special characters in message",
			code:     100,
			message:  "error: 'test' failed\n\twith \"reason\"",
			expected: "app error 100: error: 'test' failed\n\twith \"reason\"",
		},
		{
			name:     "unicode in message",
			code:     200,
			message:  "错误: 测试失败 🔥",
			expected: "app error 200: 错误: 测试失败 🔥",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &AppError{
				Code:    tt.code,
				Message: tt.message,
			}

			// Test Error() method
			result := err.Error()
			require.Equal(t, tt.expected, result)

			// Verify it implements error interface
			var _ error = err

			// Test error formatting
			formatted := fmt.Sprintf("%v", err)
			require.Equal(t, tt.expected, formatted)

			// Test error with %s
			stringFormatted := fmt.Sprintf("%s", err)
			require.Equal(t, tt.expected, stringFormatted)
		})
	}
}

func TestAppErrorAsError(t *testing.T) {
	// Test using AppError as error interface
	var err error = &AppError{
		Code:    403,
		Message: "forbidden",
	}

	require.Error(t, err)
	require.Equal(t, "app error 403: forbidden", err.Error())

	// Test type assertion
	appErr, ok := err.(*AppError)
	require.True(t, ok)
	require.Equal(t, int32(403), appErr.Code)
	require.Equal(t, "forbidden", appErr.Message)
}

func TestAppErrorComparison(t *testing.T) {
	err1 := &AppError{Code: 100, Message: "test"}
	err2 := &AppError{Code: 100, Message: "test"}
	err3 := &AppError{Code: 200, Message: "test"}
	err4 := &AppError{Code: 100, Message: "different"}

	// Same values but different instances
	require.NotSame(t, err1, err2)
	require.Equal(t, err1.Error(), err2.Error())

	// Different codes
	require.NotEqual(t, err1.Error(), err3.Error())

	// Different messages
	require.NotEqual(t, err1.Error(), err4.Error())
}

func TestAppErrorNil(t *testing.T) {
	// Test that nil AppError doesn't panic
	var err *AppError
	require.Nil(t, err)

	// Creating non-nil
	err = &AppError{Code: 1, Message: "test"}
	require.NotNil(t, err)
	require.Equal(t, "app error 1: test", err.Error())
}

func TestAppErrorInFunction(t *testing.T) {
	// Test function that returns error
	fn := func(fail bool) error {
		if fail {
			return &AppError{
				Code:    500,
				Message: "internal error",
			}
		}
		return nil
	}

	// Success case
	err := fn(false)
	require.NoError(t, err)

	// Error case
	err = fn(true)
	require.Error(t, err)
	require.Equal(t, "app error 500: internal error", err.Error())
}

func TestAppErrorWrapping(t *testing.T) {
	// Test error wrapping scenarios
	baseErr := &AppError{Code: 100, Message: "base error"}

	// Wrap with fmt.Errorf
	wrapped := fmt.Errorf("wrapped: %w", baseErr)
	require.Contains(t, wrapped.Error(), "app error 100: base error")

	// Create chain of errors
	err1 := &AppError{Code: 1, Message: "first"}
	err2 := fmt.Errorf("second: %w", err1)
	err3 := fmt.Errorf("third: %w", err2)

	require.Contains(t, err3.Error(), "first")
	require.Contains(t, err3.Error(), "second")
	require.Contains(t, err3.Error(), "third")
}

func TestAppErrorConcurrency(t *testing.T) {
	// Test concurrent access to AppError
	err := &AppError{
		Code:    999,
		Message: "concurrent",
	}

	done := make(chan bool, 100)

	// Concurrent reads
	for i := 0; i < 100; i++ {
		go func() {
			msg := err.Error()
			require.Equal(t, "app error 999: concurrent", msg)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func BenchmarkAppError_Error(b *testing.B) {
	err := &AppError{
		Code:    500,
		Message: "benchmark error message",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkAppError_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := &AppError{
			Code:    int32(i % 1000),
			Message: "error message",
		}
		_ = err.Error()
	}
}

func BenchmarkAppError_Interface(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error = &AppError{
			Code:    404,
			Message: "not found",
		}
		_ = err.Error()
	}
}

func BenchmarkAppError_TypeAssertion(b *testing.B) {
	var err error = &AppError{
		Code:    200,
		Message: "ok",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if appErr, ok := err.(*AppError); ok {
			_ = appErr.Code
		}
	}
}

// Example usage
func ExampleAppError() {
	err := &AppError{
		Code:    404,
		Message: "resource not found",
	}
	fmt.Println(err.Error())
	// Output: app error 404: resource not found
}