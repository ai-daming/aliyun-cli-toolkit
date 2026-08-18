package oss

import (
	"errors"
	"testing"
)

func TestErrorCodeRecognizesOnlyOperationErrors(t *testing.T) {
	err := newOperationError(ErrorDelete, errors.New("provider detail"))
	if code, ok := ErrorCode(err); !ok || code != ErrorDelete {
		t.Fatalf("ErrorCode = %q, %v", code, ok)
	}
	if code, ok := ErrorCode(errors.New("ordinary")); ok || code != "" {
		t.Fatalf("ordinary ErrorCode = %q, %v", code, ok)
	}
}
