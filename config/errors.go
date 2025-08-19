package config

import "errors"

var (
    ErrInvalidK           = errors.New("K must be >= 1")
    ErrInvalidAlpha       = errors.New("Alpha must be between 0.5 and 1.0")
    ErrInvalidBeta        = errors.New("Beta must be >= 1")
    ErrBlockTimeTooLow    = errors.New("BlockTime must be >= 1ms")
    ErrRoundTimeoutTooLow = errors.New("RoundTO must be >= BlockTime")
)