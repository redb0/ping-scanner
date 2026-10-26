package probe

import (
	"errors"
)

var (
	ErrEmptyTarget    = errors.New("empty target")
	ErrInvalidTarget  = errors.New("invalid target")
	ErrInvalidTimeout = errors.New("timeout must be greater than 0")
)
