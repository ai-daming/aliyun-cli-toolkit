package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadEndToEndPublic(t *testing.T) {
	pname := setupIntegrationHome(t)
	root := NewRootCmd()
	key := uniqueKey("e2e-upload-public")
	tmpFile := filepath.Join(t.TempDir(), "payload.txt")
	writeFile(t, tmpFile, []byte("e2e upload public\n"))
	t.Cleanup(func() { cleanupUploaded(t, pname, key) })

	root.SetArgs([]string{"upload", "--profile", pname, "--key", key, "--file", tmpFile, "--content-type", "text/plain"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("upload: %v\nstderr: %s", err, out.String())
	}

	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("not json: %s", out.String())
	}
	if res["visibility"] != "public" {
		t.Fatalf("visibility = %v", res["visibility"])
	}
	if res["url"] == nil || res["url"] == "" {
		t.Fatal("public upload must have url")
	}
	if res["etag"] == nil || res["etag"] == "" {
		t.Fatal("etag must be present")
	}
}

func TestUploadEndToEndPrivate(t *testing.T) {
	pname := setupIntegrationHome(t)
	root := NewRootCmd()
	key := uniqueKey("e2e-upload-private")
	tmpFile := filepath.Join(t.TempDir(), "payload.txt")
	writeFile(t, tmpFile, []byte("e2e private\n"))
	t.Cleanup(func() { cleanupUploaded(t, pname, key) })

	root.SetArgs([]string{"upload", "--profile", pname, "--key", key, "--file", tmpFile, "--content-type", "text/plain", "--private"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("upload: %v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["visibility"] != "private" {
		t.Fatalf("visibility = %v", res["visibility"])
	}
	if _, has := res["url"]; has {
		t.Fatal("private upload must not include url")
	}
}

func TestUploadMissingFile(t *testing.T) {
	pname := setupIntegrationHome(t)
	root := NewRootCmd()
	root.SetArgs([]string{"upload", "--profile", pname, "--key", "x", "--file", "/no/such/file/here"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err == nil {
		t.Fatal("upload of missing file should error")
	}
}

func TestUploadMissingProfile(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	tmpFile := filepath.Join(t.TempDir(), "p.txt")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd()
	root.SetArgs([]string{"upload", "--profile", "ghost", "--key", "x", "--file", tmpFile})
	if err := root.Execute(); err == nil {
		t.Fatal("upload with missing profile should error")
	}
}
