package media

import (
	"strings"
	"testing"
)

func TestValidateObjectInputSafeSubset(t *testing.T) {
	valid := []string{"media/2026-", "媒体/示例 photo.jpg", "a//b"}
	for _, value := range valid {
		if err := ValidateObjectInput(value); err != nil {
			t.Fatalf("ValidateObjectInput(%q): %v", value, err)
		}
	}

	invalid := []string{"", "/root", "-flag", "media/*", "media/?", ".", "media/../secret", "line\nbreak", strings.Repeat("x", MaxObjectInputBytes+1)}
	for _, value := range invalid {
		if err := ValidateObjectInput(value); err == nil {
			t.Fatalf("ValidateObjectInput(%q) should fail", value)
		}
	}
}

func TestValidateCursorLimitAndBucket(t *testing.T) {
	if err := ValidateCursor("opaque+/=token"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "bad\nvalue", strings.Repeat("x", MaxCursorBytes+1)} {
		if err := ValidateCursor(value); err == nil {
			t.Fatalf("ValidateCursor(%q) should fail", value)
		}
	}
	for _, value := range []int{1, 1000} {
		if err := ValidateLimit(value); err != nil {
			t.Fatalf("ValidateLimit(%d): %v", value, err)
		}
	}
	for _, value := range []int{0, 1001} {
		if err := ValidateLimit(value); err == nil {
			t.Fatalf("ValidateLimit(%d) should fail", value)
		}
	}
	for _, value := range []string{"component-bucket", "abc", "a1-b2"} {
		if err := ValidateBucket(value); err != nil {
			t.Fatalf("ValidateBucket(%q): %v", value, err)
		}
	}
	for _, value := range []string{"ab", "UPPERCASE", "-bucket", "bucket-", "bucket*"} {
		if err := ValidateBucket(value); err == nil {
			t.Fatalf("ValidateBucket(%q) should fail", value)
		}
	}
}
