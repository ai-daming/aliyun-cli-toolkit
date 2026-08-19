package oss

import "errors"

const (
	ErrorInvalidArgument = "INVALID_ARGUMENT"
	ErrorSTS             = "STS_ERROR"
	ErrorList            = "OSS_LIST_ERROR"
	ErrorDelete          = "OSS_DELETE_ERROR"
	ErrorInvalidResponse = "INVALID_OSS_RESPONSE"
)

type operationError struct {
	code string
}

func (e *operationError) Error() string { return e.code }

func newOperationError(code string, cause error) error {
	_ = cause // Provider details are deliberately discarded at this boundary.
	return &operationError{code: code}
}

// ErrorCode returns a stable, safe code for errors produced by list/delete.
func ErrorCode(err error) (string, bool) {
	var operationErr *operationError
	if !errors.As(err, &operationErr) {
		return "", false
	}
	return operationErr.code, true
}
