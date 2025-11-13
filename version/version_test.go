package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplication_String(t *testing.T) {
	tests := []struct {
		name     string
		version  *Application
		expected string
	}{
		{
			name: "standard version",
			version: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
				Name:  "lux",
			},
			expected: "lux-1.2.3",
		},
		{
			name: "zero version",
			version: &Application{
				Major: 0,
				Minor: 0,
				Patch: 0,
				Name:  "test",
			},
			expected: "test-0.0.0",
		},
		{
			name: "large numbers",
			version: &Application{
				Major: 999,
				Minor: 888,
				Patch: 777,
				Name:  "big",
			},
			expected: "big-999.888.777",
		},
		{
			name: "empty name",
			version: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Name:  "",
			},
			expected: "-1.0.0",
		},
		{
			name: "special characters in name",
			version: &Application{
				Major: 2,
				Minor: 1,
				Patch: 0,
				Name:  "app-name_v2",
			},
			expected: "app-name_v2-2.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestApplication_Compatible(t *testing.T) {
	tests := []struct {
		name       string
		v1         *Application
		v2         *Application
		compatible bool
	}{
		{
			name: "same major version",
			v1: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			v2: &Application{
				Major: 1,
				Minor: 3,
				Patch: 0,
			},
			compatible: true,
		},
		{
			name: "different major version",
			v1: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
			v2: &Application{
				Major: 2,
				Minor: 0,
				Patch: 0,
			},
			compatible: false,
		},
		{
			name: "exact same version",
			v1: &Application{
				Major: 3,
				Minor: 5,
				Patch: 7,
			},
			v2: &Application{
				Major: 3,
				Minor: 5,
				Patch: 7,
			},
			compatible: true,
		},
		{
			name: "zero major versions",
			v1: &Application{
				Major: 0,
				Minor: 1,
				Patch: 0,
			},
			v2: &Application{
				Major: 0,
				Minor: 2,
				Patch: 0,
			},
			compatible: true,
		},
		{
			name: "different names same major",
			v1: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Name:  "app1",
			},
			v2: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Name:  "app2",
			},
			compatible: true, // Name doesn't affect compatibility
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compatible(tt.v2)
			require.Equal(t, tt.compatible, result)

			// Test symmetry
			reverseResult := tt.v2.Compatible(tt.v1)
			require.Equal(t, tt.compatible, reverseResult)
		})
	}
}

func TestApplication_Compare(t *testing.T) {
	tests := []struct {
		name     string
		v1       *Application
		v2       *Application
		expected int
	}{
		{
			name: "v1 < v2 (major)",
			v1: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
			},
			v2: &Application{
				Major: 2,
				Minor: 0,
				Patch: 0,
			},
			expected: -1,
		},
		{
			name: "v1 > v2 (major)",
			v1: &Application{
				Major: 3,
				Minor: 0,
				Patch: 0,
			},
			v2: &Application{
				Major: 2,
				Minor: 0,
				Patch: 0,
			},
			expected: 1,
		},
		{
			name: "v1 < v2 (minor)",
			v1: &Application{
				Major: 1,
				Minor: 2,
				Patch: 0,
			},
			v2: &Application{
				Major: 1,
				Minor: 3,
				Patch: 0,
			},
			expected: -1,
		},
		{
			name: "v1 > v2 (minor)",
			v1: &Application{
				Major: 1,
				Minor: 5,
				Patch: 0,
			},
			v2: &Application{
				Major: 1,
				Minor: 3,
				Patch: 0,
			},
			expected: 1,
		},
		{
			name: "v1 < v2 (patch)",
			v1: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			v2: &Application{
				Major: 1,
				Minor: 2,
				Patch: 4,
			},
			expected: -1,
		},
		{
			name: "v1 > v2 (patch)",
			v1: &Application{
				Major: 1,
				Minor: 2,
				Patch: 5,
			},
			v2: &Application{
				Major: 1,
				Minor: 2,
				Patch: 4,
			},
			expected: 1,
		},
		{
			name: "equal versions",
			v1: &Application{
				Major: 2,
				Minor: 5,
				Patch: 8,
			},
			v2: &Application{
				Major: 2,
				Minor: 5,
				Patch: 8,
			},
			expected: 0,
		},
		{
			name: "equal versions with different names",
			v1: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Name:  "app1",
			},
			v2: &Application{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Name:  "app2",
			},
			expected: 0, // Name doesn't affect comparison
		},
		{
			name: "zero versions",
			v1: &Application{
				Major: 0,
				Minor: 0,
				Patch: 0,
			},
			v2: &Application{
				Major: 0,
				Minor: 0,
				Patch: 0,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compare(tt.v2)
			require.Equal(t, tt.expected, result)

			// Test antisymmetry
			reverseResult := tt.v2.Compare(tt.v1)
			require.Equal(t, -tt.expected, reverseResult)
		})
	}
}

func TestCurrent(t *testing.T) {
	current := Current()

	require.NotNil(t, current)
	require.Equal(t, 1, current.Major)
	require.Equal(t, 22, current.Minor)
	require.Equal(t, 0, current.Patch)
	require.Equal(t, "lux", current.Name)

	// Test string representation
	require.Equal(t, "lux-1.22.0", current.String())

	// Test that calling Current multiple times returns consistent results
	current2 := Current()
	require.Equal(t, current.Major, current2.Major)
	require.Equal(t, current.Minor, current2.Minor)
	require.Equal(t, current.Patch, current2.Patch)
	require.Equal(t, current.Name, current2.Name)
}

func TestVersionTransitivity(t *testing.T) {
	// Test transitivity of Compare
	v1 := &Application{Major: 1, Minor: 0, Patch: 0}
	v2 := &Application{Major: 2, Minor: 0, Patch: 0}
	v3 := &Application{Major: 3, Minor: 0, Patch: 0}

	// If v1 < v2 and v2 < v3, then v1 < v3
	require.Equal(t, -1, v1.Compare(v2))
	require.Equal(t, -1, v2.Compare(v3))
	require.Equal(t, -1, v1.Compare(v3))
}

func TestVersionOrdering(t *testing.T) {
	versions := []*Application{
		{Major: 1, Minor: 0, Patch: 0},
		{Major: 1, Minor: 0, Patch: 1},
		{Major: 1, Minor: 1, Patch: 0},
		{Major: 1, Minor: 1, Patch: 1},
		{Major: 2, Minor: 0, Patch: 0},
	}

	// Each version should be less than the next one
	for i := 0; i < len(versions)-1; i++ {
		result := versions[i].Compare(versions[i+1])
		require.Equal(t, -1, result,
			"Version %s should be less than %s",
			versions[i].String(),
			versions[i+1].String())
	}
}

func TestVersionReflexivity(t *testing.T) {
	// A version should be equal to itself
	v := &Application{Major: 5, Minor: 4, Patch: 3, Name: "test"}

	require.Equal(t, 0, v.Compare(v))
	require.True(t, v.Compatible(v))
}

func TestVersionEdgeCases(t *testing.T) {
	t.Run("Large version numbers", func(t *testing.T) {
		v := &Application{
			Major: 999999,
			Minor: 888888,
			Patch: 777777,
			Name:  "huge",
		}
		require.Equal(t, "huge-999999.888888.777777", v.String())
	})

	t.Run("Negative version numbers", func(t *testing.T) {
		v := &Application{
			Major: -1,
			Minor: -2,
			Patch: -3,
			Name:  "negative",
		}
		require.Equal(t, "negative--1.-2.-3", v.String())
	})
}

func BenchmarkApplication_String(b *testing.B) {
	v := &Application{
		Major: 1,
		Minor: 2,
		Patch: 3,
		Name:  "benchmark",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.String()
	}
}

func BenchmarkApplication_Compatible(b *testing.B) {
	v1 := &Application{Major: 1, Minor: 2, Patch: 3}
	v2 := &Application{Major: 1, Minor: 3, Patch: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v1.Compatible(v2)
	}
}

func BenchmarkApplication_Compare(b *testing.B) {
	v1 := &Application{Major: 1, Minor: 2, Patch: 3}
	v2 := &Application{Major: 2, Minor: 1, Patch: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v1.Compare(v2)
	}
}

func BenchmarkCurrent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Current()
	}
}
