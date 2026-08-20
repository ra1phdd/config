package config

import "errors"

var (
	ErrNilConfig           = errors.New("config is nil")
	ErrUnsafePath          = errors.New("unsafe config path")
	ErrInvalidConfig       = errors.New("invalid config")
	ErrInvalidMergeKey     = errors.New("invalid security merge key")
	ErrAmbiguousMerge      = errors.New("ambiguous security overlay merge")
	ErrInvalidFileMapKey   = errors.New("invalid file map key")
	ErrFileMapUnbound      = errors.New("file map is not bound")
	ErrFileMapConflict     = errors.New("file map key conflict")
	ErrInvalidFileMapEntry = errors.New("invalid file map entry")
)
