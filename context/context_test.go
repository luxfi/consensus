package context

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	ctx := &Context{}

	// Test that context can be created
	require.NotNil(t, ctx)
}
