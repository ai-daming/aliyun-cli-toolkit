package app

import (
	"strings"
	"testing"
)

func TestValidateObjectKeyAcceptsPortableUTF8(t *testing.T) {
	got, err := validateObjectKey("媒体/示例 photo.jpg")
	if err != nil {
		t.Fatalf("validateObjectKey: %v", err)
	}
	if got != "媒体/示例 photo.jpg" {
		t.Fatalf("key = %q", got)
	}
}

func TestValidatePrefixDoesNotRequireTrailingSlash(t *testing.T) {
	got, err := validatePrefix("media/2026-")
	if err != nil {
		t.Fatalf("validatePrefix: %v", err)
	}
	if got != "media/2026-" {
		t.Fatalf("prefix = %q", got)
	}
}

func TestValidateObjectInputRejectsUnsafeValues(t *testing.T) {
	tests := []string{
		"",
		"/absolute",
		"-option",
		"media/../secret",
		"media/./photo.jpg",
		"media/*",
		"media/?",
		"media/line\nbreak",
		strings.Repeat("a", 1024),
		strings.Repeat("界", 342), // 1026 UTF-8 bytes
		string([]byte{0xff}),
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			if _, err := validateObjectKey(value); err == nil {
				t.Fatalf("validateObjectKey(%q) should fail", value)
			}
			if _, err := validatePrefix(value); err == nil {
				t.Fatalf("validatePrefix(%q) should fail", value)
			}
		})
	}
}

func TestValidateCursorAndLimit(t *testing.T) {
	if got, err := validateCursor("opaque+/=token"); err != nil || got != "opaque+/=token" {
		t.Fatalf("validateCursor = %q, %v", got, err)
	}
	for _, value := range []string{"", "line\nbreak", strings.Repeat("x", 4097)} {
		if _, err := validateCursor(value); err == nil {
			t.Fatalf("validateCursor(%q) should fail", value)
		}
	}
	for _, value := range []string{"0", "1001", "abc", ""} {
		if _, err := validateLimit(value); err == nil {
			t.Fatalf("validateLimit(%q) should fail", value)
		}
	}
	for _, value := range []string{"1", "200", "1000"} {
		if _, err := validateLimit(value); err != nil {
			t.Fatalf("validateLimit(%q): %v", value, err)
		}
	}
}

func TestExistingKeyCommandsRejectUnsafeKeyBeforeProfileAccess(t *testing.T) {
	tests := []struct {
		name string
		cmd  commandFactory
		args []string
	}{
		{
			name: "upload",
			cmd:  newUploadCmd,
			args: []string{"--profile", "missing", "--key", "media/../secret", "--file", "/missing"},
		},
		{
			name: "resolve",
			cmd:  newResolveCmd,
			args: []string{"--profile", "missing", "--key", "media/*", "--public"},
		},
		{
			name: "stat",
			cmd:  newStatCmd,
			args: []string{"--profile", "missing", "--key", "-option"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil || err.Error() != "INVALID_ARGUMENT" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
