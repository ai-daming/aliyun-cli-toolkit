package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileAddThenShow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)

	root := NewRootCmd()

	// profile add
	root.SetArgs([]string{
		"profile", "add", "testprof",
		"--bucket", "bkt",
		"--region", "cn-huhehaote",
		"--endpoint", "oss-cn-huhehaote.aliyuncs.com",
		"--role-arn", "acs:ram::1:role/r",
		"--access-key-id", "THEAK",
		"--access-key-secret", "THESK",
		"--public-domain", "cdn.x.com",
	})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v", err)
	}

	// verify file written
	data, err := os.ReadFile(filepath.Join(dir, "profiles", "testprof.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("THEAK")) {
		t.Fatalf("ak not persisted: %s", data)
	}

	// profile show (masked)
	root2 := NewRootCmd()
	root2.SetArgs([]string{"profile", "show", "testprof"})
	out2 := &bytes.Buffer{}
	root2.SetOut(out2)
	root2.SetErr(out2)
	if err := root2.Execute(); err != nil {
		t.Fatalf("show: %v", err)
	}
	var shown map[string]any
	if err := json.Unmarshal(out2.Bytes(), &shown); err != nil {
		t.Fatalf("show output not json: %s\n%v", out2.String(), err)
	}
	if shown["access-key-secret"] != "***" {
		t.Fatalf("show should mask secret, got %v", shown["access-key-secret"])
	}
	if shown["access-key-id"] != "THEAK" {
		t.Fatalf("AKID should be visible, got %v", shown["access-key-id"])
	}
}

func TestProfileList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	for _, n := range []string{"a", "b"} {
		runAdd(t, n)
	}
	root := NewRootCmd()
	root.SetArgs([]string{"profile", "list"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var res struct {
		Profiles []string `json:"profiles"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("list output: %s\n%v", out.String(), err)
	}
	if len(res.Profiles) != 2 || res.Profiles[0] != "a" || res.Profiles[1] != "b" {
		t.Fatalf("list = %+v", res)
	}
}

func TestProfileRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	runAdd(t, "victim")
	root := NewRootCmd()
	root.SetArgs([]string{"profile", "remove", "victim"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// listing should now be empty
	root2 := NewRootCmd()
	root2.SetArgs([]string{"profile", "list"})
	out2 := &bytes.Buffer{}
	root2.SetOut(out2)
	_ = root2.Execute()
	var res struct {
		Profiles []string `json:"profiles"`
	}
	_ = json.Unmarshal(out2.Bytes(), &res)
	if len(res.Profiles) != 0 {
		t.Fatalf("expected empty list after remove, got %+v", res)
	}
}

func TestProfileShowMissing(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	root := NewRootCmd()
	root.SetArgs([]string{"profile", "show", "ghost"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err == nil {
		t.Fatal("show of missing profile should error")
	}
}

func runAdd(t *testing.T, name string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"profile", "add", name, "--bucket", "b", "--region", "r", "--endpoint", "e", "--role-arn", "ra", "--access-key-id", "ak", "--access-key-secret", "sk"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
