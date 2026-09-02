package incident

import "errors"

var (
	ErrNotFound               = errors.New("incident not found")
	ErrInvalidTransition      = errors.New("invalid status transition")
	ErrInvalidInput           = errors.New("invalid incident input")
	ErrConcurrentModification = errors.New("incident was modified concurrently")
)

// Field length limits for input validation.
const (
	MaxTitleLength       = 500
	MaxDescriptionLength = 10_000
	MaxServiceLength     = 200
	MaxEnvironmentLength = 100
	MaxAlertSourceLength = 100
)
