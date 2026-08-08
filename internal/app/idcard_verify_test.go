package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setupIdcardIntegrationHome copies profiles.local/<name>.toml into a temp
// profiles/ dir. Skips if absent.
func setupIdcardIntegrationHome(t *testing.T) string {
	t.Helper()
	name := os.Getenv("ALIYUN_IDCARD_TEST_PROFILE")
	if name == "" {
		name = "mamamate-verify"
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(repoRoot, "profiles.local", name+".toml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("integration profile %q not found at %s: %v", name, src, err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "profiles", name+".toml")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	return name
}

// TestIdcardVerifyRoundTrip exercises the verify subcommand end-to-end against
// real CloudAuth, using a bogus image URL. The call reaches CloudAuth (HTTP
// round-trip), so the full cobra → profile → verifier → JSON path is covered.
// Result may be ok=false or passed=false, but the command must produce JSON.
func TestIdcardVerifyRoundTrip(t *testing.T) {
	pname := setupIdcardIntegrationHome(t)
	root := NewIdcardRootCmd()
	root.SetArgs([]string{
		"verify",
		"--profile", pname,
		"--front-url", "https://example.com/no-such-idcard-" + uniqueKey("idc") + ".jpg",
	})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	_ = root.Execute() // may error (API rejects bogus image) — that's fine

	var res map[string]any
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("verify must emit JSON, got: %s\n%v", out.String(), err)
	}
	// ok may be true or false; just ensure it's present
	if _, has := res["ok"]; !has {
		t.Fatalf("verify result missing 'ok': %s", out.String())
	}
}

func TestIdcardProfileAddShow(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	root := NewIdcardRootCmd()
	root.SetArgs([]string{"profile", "add", "vp", "--region", "cn-shanghai", "--endpoint", "cloudauth.cn-shanghai.aliyuncs.com", "--access-key-id", "AK", "--access-key-secret", "SK"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root2 := NewIdcardRootCmd()
	root2.SetArgs([]string{"profile", "show", "vp"})
	out := &bytes.Buffer{}
	root2.SetOut(out)
	if err := root2.Execute(); err != nil {
		t.Fatal(err)
	}
	var shown map[string]any
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil {
		t.Fatal(err)
	}
	if shown["access-key-secret"] != "***" {
		t.Fatalf("secret not masked: %v", shown["access-key-secret"])
	}
}

func TestIdcardVerifyMissingProfile(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	root := NewIdcardRootCmd()
	root.SetArgs([]string{"verify", "--profile", "ghost", "--front-url", "https://x/y.jpg"})
	if err := root.Execute(); err == nil {
		t.Fatal("verify with missing profile should error")
	}
}

func TestIdcardProfileListAndRemove(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	// add two profiles
	for _, n := range []string{"p1", "p2"} {
		root := NewIdcardRootCmd()
		root.SetArgs([]string{"profile", "add", n, "--region", "r", "--access-key-id", "a", "--access-key-secret", "s"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	// list
	root := NewIdcardRootCmd()
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
		t.Fatal(err)
	}
	if len(res.Profiles) != 2 {
		t.Fatalf("list = %+v", res)
	}
	// remove one
	root2 := NewIdcardRootCmd()
	root2.SetArgs([]string{"profile", "remove", "p1"})
	if err := root2.Execute(); err != nil {
		t.Fatal(err)
	}
	// list again — should have one left
	root3 := NewIdcardRootCmd()
	root3.SetArgs([]string{"profile", "list"})
	out3 := &bytes.Buffer{}
	root3.SetOut(out3)
	if err := root3.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out3.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 1 || res.Profiles[0] != "p2" {
		t.Fatalf("after remove, list = %+v", res)
	}
}
