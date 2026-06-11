package config

import "errors"

var (
	ErrNilConfig     = errors.New("config is nil")
	ErrUnsafePath    = errors.New("unsafe config path")
	ErrInvalidConfig = errors.New("invalid config")
)
