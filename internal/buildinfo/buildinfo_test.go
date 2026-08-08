package buildinfo

import "testing"

func TestDefaults(t *testing.T) {
	// buildinfo must always have non-panicking defaults
	if Version == "" {
		t.Fatal("Version default should be set, got empty")
	}
	if Commit == "" {
		t.Fatal("Commit default should be set, got empty")
	}
	if BuildTime == "" {
		t.Fatal("BuildTime default should be set, got empty")
	}
}
