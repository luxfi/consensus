package core

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageType_String(t *testing.T) {
	tests := []struct {
		name        string
		messageType MessageType
		expected    string
	}{
		{
			name:        "PendingTxs",
			messageType: PendingTxs,
			expected:    "PendingTxs",
		},
		{
			name:        "PutBlock",
			messageType: PutBlock,
			expected:    "PutBlock",
		},
		{
			name:        "GetBlock",
			messageType: GetBlock,
			expected:    "GetBlock",
		},
		{
			name:        "GetAccepted",
			messageType: GetAccepted,
			expected:    "GetAccepted",
		},
		{
			name:        "Accepted",
			messageType: Accepted,
			expected:    "Accepted",
		},
		{
			name:        "GetAncestors",
			messageType: GetAncestors,
			expected:    "GetAncestors",
		},
		{
			name:        "MultiPut",
			messageType: MultiPut,
			expected:    "MultiPut",
		},
		{
			name:        "GetFailed",
			messageType: GetFailed,
			expected:    "GetFailed",
		},
		{
			name:        "QueryFailed",
			messageType: QueryFailed,
			expected:    "QueryFailed",
		},
		{
			name:        "Chits",
			messageType: Chits,
			expected:    "Chits",
		},
		{
			name:        "ChitsV2",
			messageType: ChitsV2,
			expected:    "ChitsV2",
		},
		{
			name:        "GetAcceptedFrontier",
			messageType: GetAcceptedFrontier,
			expected:    "GetAcceptedFrontier",
		},
		{
			name:        "AcceptedFrontier",
			messageType: AcceptedFrontier,
			expected:    "AcceptedFrontier",
		},
		{
			name:        "GetAcceptedFrontierFailed",
			messageType: GetAcceptedFrontierFailed,
			expected:    "GetAcceptedFrontierFailed",
		},
		{
			name:        "WarpRequest",
			messageType: WarpRequest,
			expected:    "WarpRequest",
		},
		{
			name:        "WarpResponse",
			messageType: WarpResponse,
			expected:    "WarpResponse",
		},
		{
			name:        "WarpGossip",
			messageType: WarpGossip,
			expected:    "WarpGossip",
		},
		{
			name:        "StateSyncDone",
			messageType: StateSyncDone,
			expected:    "StateSyncDone",
		},
		{
			name:        "Unknown type (large positive)",
			messageType: MessageType(9999),
			expected:    "Unknown",
		},
		{
			name:        "Unknown type (max uint32)",
			messageType: MessageType(^uint32(0)),
			expected:    "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.messageType.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMessageType_Constants(t *testing.T) {
	// Verify constant ordering (iota values)
	require.Equal(t, MessageType(0), PendingTxs)
	require.Equal(t, MessageType(1), PutBlock)
	require.Equal(t, MessageType(2), GetBlock)
	require.Equal(t, MessageType(3), GetAccepted)
	require.Equal(t, MessageType(4), Accepted)
	require.Equal(t, MessageType(5), GetAncestors)
	require.Equal(t, MessageType(6), MultiPut)
	require.Equal(t, MessageType(7), GetFailed)
	require.Equal(t, MessageType(8), QueryFailed)
	require.Equal(t, MessageType(9), Chits)
	require.Equal(t, MessageType(10), ChitsV2)
	require.Equal(t, MessageType(11), GetAcceptedFrontier)
	require.Equal(t, MessageType(12), AcceptedFrontier)
	require.Equal(t, MessageType(13), GetAcceptedFrontierFailed)
	require.Equal(t, MessageType(14), WarpRequest)
	require.Equal(t, MessageType(15), WarpResponse)
	require.Equal(t, MessageType(16), WarpGossip)
	require.Equal(t, MessageType(17), StateSyncDone)
}

func TestMessageType_AllKnownTypes(t *testing.T) {
	knownTypes := []MessageType{
		PendingTxs,
		PutBlock,
		GetBlock,
		GetAccepted,
		Accepted,
		GetAncestors,
		MultiPut,
		GetFailed,
		QueryFailed,
		Chits,
		ChitsV2,
		GetAcceptedFrontier,
		AcceptedFrontier,
		GetAcceptedFrontierFailed,
		WarpRequest,
		WarpResponse,
		WarpGossip,
		StateSyncDone,
	}

	for _, mt := range knownTypes {
		result := mt.String()
		require.NotEqual(t, "Unknown", result, "MessageType %d should not be Unknown", mt)
	}
}

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name: "standard error",
			err: &AppError{
				Code:    100,
				Message: "something went wrong",
			},
			expected: "app error 100: something went wrong",
		},
		{
			name: "zero code",
			err: &AppError{
				Code:    0,
				Message: "zero code error",
			},
			expected: "app error 0: zero code error",
		},
		{
			name: "negative code",
			err: &AppError{
				Code:    -1,
				Message: "negative code error",
			},
			expected: "app error -1: negative code error",
		},
		{
			name: "empty message",
			err: &AppError{
				Code:    500,
				Message: "",
			},
			expected: "app error 500: ",
		},
		{
			name: "large code",
			err: &AppError{
				Code:    2147483647,
				Message: "max int32 code",
			},
			expected: "app error 2147483647: max int32 code",
		},
		{
			name: "min int32 code",
			err: &AppError{
				Code:    -2147483648,
				Message: "min int32 code",
			},
			expected: "app error -2147483648: min int32 code",
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestAppError_Interface(t *testing.T) {
	// Verify AppError implements error interface
	var _ error = (*AppError)(nil)

	err := &AppError{
		Code:    404,
		Message: "not found",
	}

	var e error = err
	require.Equal(t, "app error 404: not found", e.Error())
}

func TestAppError_Nil(t *testing.T) {
	var err *AppError
	require.Equal(t, "", err.Error())
}

func TestMessage_Struct(t *testing.T) {
	msg := Message{
		Type:    PendingTxs,
		Content: []byte("test content"),
	}

	require.Equal(t, PendingTxs, msg.Type)
	require.Equal(t, []byte("test content"), msg.Content)
	require.Equal(t, "PendingTxs", msg.Type.String())
}

func TestVMState_Values(t *testing.T) {
	// Verify VMState constants
	require.Equal(t, VMState(0), VMInitializing)
	require.Equal(t, VMState(1), VMStateSyncing)
	require.Equal(t, VMState(2), VMBootstrapping)
	require.Equal(t, VMState(3), VMNormalOp)
}

func TestFx_Struct(t *testing.T) {
	fx := Fx{
		Fx: "test fx",
	}

	require.Equal(t, "test fx", fx.Fx)
}

// Benchmarks
func BenchmarkMessageType_String(b *testing.B) {
	messageTypes := []MessageType{
		PendingTxs, PutBlock, GetBlock, GetAccepted, Accepted,
		GetAncestors, MultiPut, GetFailed, QueryFailed, Chits,
		ChitsV2, GetAcceptedFrontier, AcceptedFrontier, GetAcceptedFrontierFailed,
		WarpRequest, WarpResponse, WarpGossip, StateSyncDone,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, mt := range messageTypes {
			_ = mt.String()
		}
	}
}

func BenchmarkAppError_Error(b *testing.B) {
	err := &AppError{
		Code:    500,
		Message: "internal server error",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

func BenchmarkAppError_Error_Nil(b *testing.B) {
	var err *AppError

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// Examples
func ExampleMessageType_String() {
	fmt.Println(PendingTxs.String())
	fmt.Println(GetBlock.String())
	fmt.Println(WarpRequest.String())
	// Output:
	// PendingTxs
	// GetBlock
	// WarpRequest
}

func ExampleAppError_Error() {
	err := &AppError{
		Code:    404,
		Message: "resource not found",
	}
	fmt.Println(err.Error())
	// Output: app error 404: resource not found
}
