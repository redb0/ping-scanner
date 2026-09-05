package probe

import (
	"errors"
)

var (
	ErrEmptyTarget   = errors.New("empty target")
	ErrInvalidTarget = errors.New("invalid target")
)
