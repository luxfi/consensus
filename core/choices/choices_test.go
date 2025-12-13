package choices

import (
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
			name:     "Unknown status",
			status:   Unknown,
			expected: "Unknown",
		},
		{
			name:     "Processing status",
			status:   Processing,
			expected: "Processing",
		},
		{
			name:     "Rejected status",
			status:   Rejected,
			expected: "Rejected",
		},
		{
			name:     "Accepted status",
			status:   Accepted,
			expected: "Accepted",
		},
		{
			name:     "Invalid status",
			status:   Status(99),
			expected: "Invalid",
		},
		{
			name:     "Max uint8 status",
			status:   Status(255),
			expected: "Invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestStatusConstants(t *testing.T) {
	// Verify the constant values
	require.Equal(t, Status(0), Unknown)
	require.Equal(t, Status(1), Processing)
	require.Equal(t, Status(2), Rejected)
	require.Equal(t, Status(3), Accepted)
}

func TestStatusComparison(t *testing.T) {
	// Test equality (using variables to avoid trivial comparison warnings)
	unknown := Unknown
	processing := Processing
	rejected := Rejected
	accepted := Accepted
	require.Equal(t, Unknown, unknown)
	require.Equal(t, Processing, processing)
	require.Equal(t, Rejected, rejected)
	require.Equal(t, Accepted, accepted)

	// Test inequality
	require.True(t, Unknown != Processing)
	require.True(t, Unknown != Rejected)
	require.True(t, Unknown != Accepted)
	require.True(t, Processing != Rejected)
	require.True(t, Processing != Accepted)
	require.True(t, Rejected != Accepted)

	// Test ordering
	require.True(t, Unknown < Processing)
	require.True(t, Processing < Rejected)
	require.True(t, Rejected < Accepted)
}

func TestStatusTypeAssertion(t *testing.T) {
	var s interface{} = Unknown

	// Should be able to type assert to Status
	status, ok := s.(Status)
	require.True(t, ok)
	require.Equal(t, Unknown, status)

	// Should be uint8 under the hood
	require.Equal(t, uint8(0), uint8(Unknown))
	require.Equal(t, uint8(1), uint8(Processing))
	require.Equal(t, uint8(2), uint8(Rejected))
	require.Equal(t, uint8(3), uint8(Accepted))
}

func TestStatusZeroValue(t *testing.T) {
	var s Status
	// Zero value should be Unknown
	require.Equal(t, Unknown, s)
	require.Equal(t, "Unknown", s.String())
}

func TestStatusSwitch(t *testing.T) {
	statuses := []Status{Unknown, Processing, Rejected, Accepted, Status(100)}

	for _, s := range statuses {
		// Test that switch statement works correctly
		var result string
		switch s {
		case Unknown:
			result = "u"
		case Processing:
			result = "p"
		case Rejected:
			result = "r"
		case Accepted:
			result = "a"
		default:
			result = "i"
		}

		switch s {
		case Unknown:
			require.Equal(t, "u", result)
		case Processing:
			require.Equal(t, "p", result)
		case Rejected:
			require.Equal(t, "r", result)
		case Accepted:
			require.Equal(t, "a", result)
		default:
			require.Equal(t, "i", result)
		}
	}
}

func BenchmarkStatus_String(b *testing.B) {
	statuses := []Status{Unknown, Processing, Rejected, Accepted, Status(99)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = statuses[i%len(statuses)].String()
	}
}

func BenchmarkStatusComparison(b *testing.B) {
	s1 := Processing
	s2 := Accepted

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s1 == s2
	}
}

func ExampleStatus_String() {
	s := Accepted
	println(s.String())
	// Output would be: Accepted
}
