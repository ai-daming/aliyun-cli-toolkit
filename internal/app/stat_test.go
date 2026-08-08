package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStatExistsEndToEnd(t *testing.T) {
	pname := setupIntegrationHome(t)
	key := uniqueKey("e2e-stat")
	tmpFile := filepath.Join(t.TempDir(), "s.txt")
	writeFile(t, tmpFile, []byte("stateme\n"))
	uploadKey(t, pname, key, tmpFile, "text/plain", false)
	t.Cleanup(func() { cleanupUploaded(t, pname, key) })

	root := NewRootCmd()
	root.SetArgs([]string{"stat", "--profile", pname, "--key", key})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("stat: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["exists"] != true {
		t.Fatalf("exists = %v", res["exists"])
	}
	if res["etag"] == nil || res["etag"] == "" {
		t.Fatal("etag missing")
	}
}

func TestStatMissingEndToEnd(t *testing.T) {
	pname := setupIntegrationHome(t)
	root := NewRootCmd()
	root.SetArgs([]string{"stat", "--profile", pname, "--key", uniqueKey("does-not-exist")})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("stat missing should not error: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["exists"] != false {
		t.Fatalf("exists = %v, want false", res["exists"])
	}
}
