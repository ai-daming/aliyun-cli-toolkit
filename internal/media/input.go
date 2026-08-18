// Package media owns the input boundaries shared by CLI commands and OSS
// policy construction. Keeping them here prevents a caller from bypassing the
// CLI and accidentally broadening a temporary STS policy.
package media

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	MaxObjectInputBytes = 1023
	MaxCursorBytes      = 4096
)

var ErrInvalidInput = errors.New("invalid media input")

func ValidateBucket(value string) error {
	if len(value) < 3 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return ErrInvalidInput
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return ErrInvalidInput
		}
	}
	return nil
}

func ValidateObjectInput(value string) error {
	if value == "" || len(value) > MaxObjectInputBytes || !utf8.ValidString(value) ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "-") ||
		strings.ContainsAny(value, "*?") {
		return ErrInvalidInput
	}
	for _, r := range value {
		if r <= 0x1f || r == 0x7f {
			return ErrInvalidInput
		}
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return ErrInvalidInput
		}
	}
	return nil
}

func ValidateCursor(value string) error {
	if value == "" || len(value) > MaxCursorBytes || !utf8.ValidString(value) {
		return ErrInvalidInput
	}
	for _, r := range value {
		if r <= 0x1f || r == 0x7f {
			return ErrInvalidInput
		}
	}
	return nil
}

func ValidateLimit(value int) error {
	if value < 1 || value > 1000 {
		return ErrInvalidInput
	}
	return nil
}
