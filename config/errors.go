package config

import "errors"

var (
	ErrInvalidK           = errors.New("k must be >= 1")
	ErrInvalidAlpha       = errors.New("alpha must be between 0.5 and 1.0")
	ErrInvalidBeta        = errors.New("beta must be >= 1")
	ErrBlockTimeTooLow    = errors.New("block time must be >= 1ms")
	ErrRoundTimeoutTooLow = errors.New("round timeout must be >= block time")
)
