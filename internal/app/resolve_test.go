package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestResolvePublicEndToEnd(t *testing.T) {
	pname := setupIntegrationHome(t)
	key := uniqueKey("e2e-resolve-public")
	tmpFile := filepath.Join(t.TempDir(), "p.txt")
	writeFile(t, tmpFile, []byte("resolve public\n"))
	uploadKey(t, pname, key, tmpFile, "text/plain", false)
	t.Cleanup(func() { cleanupUploaded(t, pname, key) })

	root := NewRootCmd()
	root.SetArgs([]string{"resolve", "--profile", pname, "--key", key, "--public"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["visibility"] != "public" {
		t.Fatalf("visibility = %v", res["visibility"])
	}
	if res["url"] == nil {
		t.Fatal("missing url")
	}
}

func TestResolvePrivateEndToEnd(t *testing.T) {
	pname := setupIntegrationHome(t)
	key := uniqueKey("e2e-resolve-private")
	tmpFile := filepath.Join(t.TempDir(), "p.txt")
	writeFile(t, tmpFile, []byte("resolve private\n"))
	uploadKey(t, pname, key, tmpFile, "text/plain", true)
	t.Cleanup(func() { cleanupUploaded(t, pname, key) })

	root := NewRootCmd()
	root.SetArgs([]string{"resolve", "--profile", pname, "--key", key, "--ttl", "10m"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["visibility"] != "private" {
		t.Fatalf("visibility = %v", res["visibility"])
	}
	url, _ := res["url"].(string)
	if url == "" {
		t.Fatal("signed url empty")
	}
	if res["expiresAt"] == nil {
		t.Fatal("private resolve must include expiresAt")
	}
}
