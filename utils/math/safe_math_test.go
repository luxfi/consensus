// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package math

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdd64(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
		err  error
	}{
		{
			name: "normal addition",
			a:    10,
			b:    20,
			want: 30,
			err:  nil,
		},
		{
			name: "zero addition",
			a:    0,
			b:    0,
			want: 0,
			err:  nil,
		},
		{
			name: "max value",
			a:    math.MaxUint64 - 1,
			b:    1,
			want: math.MaxUint64,
			err:  nil,
		},
		{
			name: "overflow",
			a:    math.MaxUint64,
			b:    1,
			want: 0,
			err:  ErrOverflow,
		},
		{
			name: "overflow both large",
			a:    math.MaxUint64 - 10,
			b:    20,
			want: 0,
			err:  ErrOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Add64(tt.a, tt.b)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestSub64(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
		err  error
	}{
		{
			name: "normal subtraction",
			a:    30,
			b:    20,
			want: 10,
			err:  nil,
		},
		{
			name: "zero subtraction",
			a:    10,
			b:    0,
			want: 10,
			err:  nil,
		},
		{
			name: "equal values",
			a:    100,
			b:    100,
			want: 0,
			err:  nil,
		},
		{
			name: "underflow",
			a:    10,
			b:    20,
			want: 0,
			err:  ErrUnderflow,
		},
		{
			name: "underflow from zero",
			a:    0,
			b:    1,
			want: 0,
			err:  ErrUnderflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sub64(tt.a, tt.b)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestMul64(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
		err  error
	}{
		{
			name: "normal multiplication",
			a:    10,
			b:    20,
			want: 200,
			err:  nil,
		},
		{
			name: "multiply by zero",
			a:    100,
			b:    0,
			want: 0,
			err:  nil,
		},
		{
			name: "multiply by one",
			a:    100,
			b:    1,
			want: 100,
			err:  nil,
		},
		{
			name: "max safe multiplication",
			a:    math.MaxUint64 / 2,
			b:    2,
			want: math.MaxUint64 - 1,
			err:  nil,
		},
		{
			name: "overflow",
			a:    math.MaxUint64,
			b:    2,
			want: 0,
			err:  ErrOverflow,
		},
		{
			name: "overflow large values",
			a:    math.MaxUint64 / 2,
			b:    3,
			want: 0,
			err:  ErrOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Mul64(tt.a, tt.b)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"a smaller", 1, 2, 1},
		{"b smaller", 2, 1, 1},
		{"equal", 5, 5, 5},
		{"negative values", -5, -2, -5},
		{"mixed signs", -5, 2, -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Min(tt.a, tt.b))
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"a larger", 2, 1, 2},
		{"b larger", 1, 2, 2},
		{"equal", 5, 5, 5},
		{"negative values", -5, -2, -2},
		{"mixed signs", -5, 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Max(tt.a, tt.b))
		})
	}
}

func TestMin64(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
	}{
		{"a smaller", 1, 2, 1},
		{"b smaller", 2, 1, 1},
		{"equal", 5, 5, 5},
		{"zero", 0, 100, 0},
		{"max value", math.MaxUint64, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Min64(tt.a, tt.b))
		})
	}
}

func TestMax64(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
	}{
		{"a larger", 2, 1, 2},
		{"b larger", 1, 2, 2},
		{"equal", 5, 5, 5},
		{"zero", 0, 100, 100},
		{"max value", math.MaxUint64, 100, math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Max64(tt.a, tt.b))
		})
	}
}

func TestAbsDiff(t *testing.T) {
	tests := []struct {
		name string
		a, b uint64
		want uint64
	}{
		{"a larger", 10, 3, 7},
		{"b larger", 3, 10, 7},
		{"equal", 5, 5, 0},
		{"zero diff", 0, 0, 0},
		{"max diff", math.MaxUint64, 0, math.MaxUint64},
		{"near values", 100, 99, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AbsDiff(tt.a, tt.b))
		})
	}
}

func TestAliases(t *testing.T) {
	// Test Add alias
	t.Run("Add alias", func(t *testing.T) {
		result1, err1 := Add(10, 20)
		result2, err2 := Add64(10, 20)
		require.Equal(t, result1, result2)
		require.Equal(t, err1, err2)

		// Test overflow
		_, err3 := Add(math.MaxUint64, 1)
		_, err4 := Add64(math.MaxUint64, 1)
		require.ErrorIs(t, err3, ErrOverflow)
		require.ErrorIs(t, err4, ErrOverflow)
	})

	// Test Sub alias
	t.Run("Sub alias", func(t *testing.T) {
		result1, err1 := Sub(30, 20)
		result2, err2 := Sub64(30, 20)
		require.Equal(t, result1, result2)
		require.Equal(t, err1, err2)

		// Test underflow
		_, err3 := Sub(10, 20)
		_, err4 := Sub64(10, 20)
		require.ErrorIs(t, err3, ErrUnderflow)
		require.ErrorIs(t, err4, ErrUnderflow)
	})

	// Test Mul alias
	t.Run("Mul alias", func(t *testing.T) {
		result1, err1 := Mul(10, 20)
		result2, err2 := Mul64(10, 20)
		require.Equal(t, result1, result2)
		require.Equal(t, err1, err2)

		// Test overflow
		_, err3 := Mul(math.MaxUint64, 2)
		_, err4 := Mul64(math.MaxUint64, 2)
		require.ErrorIs(t, err3, ErrOverflow)
		require.ErrorIs(t, err4, ErrOverflow)
	})
}
