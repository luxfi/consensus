package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{
			name:     "pending status",
			status:   StatusPending,
			expected: "pending",
		},
		{
			name:     "processing status",
			status:   StatusProcessing,
			expected: "processing",
		},
		{
			name:     "accepted status",
			status:   StatusAccepted,
			expected: "accepted",
		},
		{
			name:     "rejected status",
			status:   StatusRejected,
			expected: "rejected",
		},
		{
			name:     "unknown status (zero value)",
			status:   StatusUnknown,
			expected: "unknown",
		},
		{
			name:     "invalid positive status",
			status:   Status(100),
			expected: "unknown",
		},
		{
			name:     "invalid negative status",
			status:   Status(-1),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestStatus_Decided(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected bool
	}{
		{
			name:     "pending is not decided",
			status:   StatusPending,
			expected: false,
		},
		{
			name:     "processing is not decided",
			status:   StatusProcessing,
			expected: false,
		},
		{
			name:     "accepted is decided",
			status:   StatusAccepted,
			expected: true,
		},
		{
			name:     "rejected is decided",
			status:   StatusRejected,
			expected: true,
		},
		{
			name:     "unknown is not decided",
			status:   StatusUnknown,
			expected: false,
		},
		{
			name:     "invalid status is not decided",
			status:   Status(999),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.Decided()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestStatus_Constants(t *testing.T) {
	// Verify constant ordering (iota values)
	require.Equal(t, Status(0), StatusUnknown)
	require.Equal(t, Status(1), StatusPending)
	require.Equal(t, Status(2), StatusProcessing)
	require.Equal(t, Status(3), StatusAccepted)
	require.Equal(t, Status(4), StatusRejected)
}

func TestStatus_AllDecidedStates(t *testing.T) {
	decidedStates := []Status{StatusAccepted, StatusRejected}
	undecidedStates := []Status{StatusUnknown, StatusPending, StatusProcessing}

	for _, s := range decidedStates {
		require.True(t, s.Decided(), "expected %s to be decided", s.String())
	}

	for _, s := range undecidedStates {
		require.False(t, s.Decided(), "expected %s to not be decided", s.String())
	}
}

// Benchmarks
func BenchmarkStatus_String(b *testing.B) {
	statuses := []Status{StatusUnknown, StatusPending, StatusProcessing, StatusAccepted, StatusRejected}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range statuses {
			_ = s.String()
		}
	}
}

func BenchmarkStatus_Decided(b *testing.B) {
	statuses := []Status{StatusUnknown, StatusPending, StatusProcessing, StatusAccepted, StatusRejected}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range statuses {
			_ = s.Decided()
		}
	}
}

// Examples
func ExampleStatus_String() {
	fmt.Println(StatusPending.String())
	fmt.Println(StatusAccepted.String())
	// Output:
	// pending
	// accepted
}

func ExampleStatus_Decided() {
	fmt.Println(StatusPending.Decided())
	fmt.Println(StatusAccepted.Decided())
	fmt.Println(StatusRejected.Decided())
	// Output:
	// false
	// true
	// true
}
