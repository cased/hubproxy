package storage

import "errors"

// Common storage errors
var (
	ErrDuplicateKey = errors.New("duplicate key")
)

// IsDuplicateKeyError returns true if the error is a duplicate key error
func IsDuplicateKeyError(err error) bool {
	return errors.Is(err, ErrDuplicateKey)
}
