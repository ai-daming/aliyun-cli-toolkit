package app

import (
	"strconv"

	"github.com/mamamate/aliyun-cli-toolkit/internal/media"
)

type safeCommandError string

func (e safeCommandError) Error() string { return string(e) }

func commandError(code string) error { return safeCommandError(code) }

func validateProfileName(value string) (string, error) {
	if value == "" || len(value) > 128 || value[0] == '-' {
		return "", commandError("INVALID_ARGUMENT")
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return "", commandError("INVALID_ARGUMENT")
		}
	}
	return value, nil
}

func validateObjectKey(value string) (string, error) {
	return validateObjectInput(value)
}

func validatePrefix(value string) (string, error) {
	return validateObjectInput(value)
}

func validateObjectInput(value string) (string, error) {
	if err := media.ValidateObjectInput(value); err != nil {
		return "", commandError("INVALID_ARGUMENT")
	}
	return value, nil
}

func validateCursor(value string) (string, error) {
	if err := media.ValidateCursor(value); err != nil {
		return "", commandError("INVALID_ARGUMENT")
	}
	return value, nil
}

func validateLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || media.ValidateLimit(limit) != nil {
		return 0, commandError("INVALID_ARGUMENT")
	}
	return limit, nil
}
