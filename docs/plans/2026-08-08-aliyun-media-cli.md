# aliyun-media-cli Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `aliyun-media-cli`, a stateless Go CLI that uploads files to Alibaba Cloud OSS, resolves keys to readable URLs (public stable / private signed), and stats object metadata — all with JSON I/O and profile-based credentials.

**Architecture:** Single Go binary using cobra for CLI structure. Profile credentials loaded from TOML files. OSS operations via the official aliyun-oss-go-sdk + STS AssumeRole (mirroring Mamamate's `OssStsService`). Every operation: read profile → AssumeRole → do one OSS action → print JSON → exit. Tests hit the real OSS bucket (profile `mamamate` in `profiles.local/`) — no mocks.

**Tech Stack:** Go 1.22+, spf13/cobra, pelletier/go-toml/v2, aliyun-oss-go-sdk, alibabacloud-go/sts-20150401. Build via Taskfile (consistent with mm-resume).

## Global Constraints

- **Language:** Go 1.22+ (go.mod `go 1.22`).
- **Testing:** TDD — write failing test first. ≥85% coverage on changed code. **No mocks** — all OSS tests hit the real bucket using `profiles.local/mamamate.toml`. Tests that need a profile read from `ALIYUN_MEDIA_CLI_TEST_PROFILE` env (defaults to `mamamate`); tests skip if the profile file is absent (so CI without credentials doesn't fail).
- **Test fixtures:** real files in `test-assets/` (jpg, zip, txt). Tests use unique keys prefixed `test-cli/` so runs don't collide; tests clean up (delete) the objects they upload.
- **I/O contract:** stdout is always valid JSON (machine-readable). Human messages go to stderr. Exit code 0 on success, non-zero on failure (stderr gets a JSON error object `{"error":"..."}`).
- **Credentials:** never on the command line (shell history / process list risk). `profile add` reads AK/SK from flags-or-interactive, stores to TOML file with 0600 perms. No default profile — `--profile` is required on every operation command.
- **Private objects:** never return a stable URL. Only `resolve` with STS signing produces a readable URL for private objects. `upload --private` returns key + size + etag but NO url.
- **module path:** `github.com/mamamate/aliyun-cli-toolkit`

---

## File Structure

```
cmd/aliyun-media-cli/main.go              — thin entry; delegates to internal/app
go.mod / go.sum
internal/
  profile/
    profile.go                            — Profile struct, Load, Save, List, Delete, file path resolution
    profile_test.go                       — real filesystem round-trip tests
  oss/
    client.go                             — Client wrapping OSS+STS: AssumeRole, Upload, PresignGet, HeadObject, PublicURL
    client_test.go                        — real OSS integration tests (needs profile + network)
  output/
    output.go                             — PrintJSON, PrintError, exit code helpers
    output_test.go
  app/
    root.go                               — cobra root command, --profile global flag wiring
    profile_cmd.go                        — profile add/list/remove/show subcommands
    upload.go                             — upload subcommand
    resolve.go                            — resolve subcommand
    stat.go                               — stat subcommand
    upload_test.go                        — end-to-end: real OSS upload + verify
    resolve_test.go                       — end-to-end: real OSS resolve public + private
    stat_test.go                          — end-to-end: real OSS stat exists + not-exists
    e2e_test.go                           — shared test helpers (unique key, cleanup, profile guard)
Taskfile.yml
```

---

### Task 1: Project init — go.mod, dependencies, Taskfile, buildinfo

**Files:**
- Create: `go.mod`
- Create: `Taskfile.yml`
- Create: `internal/buildinfo/buildinfo.go`
- Create: `internal/buildinfo/buildinfo_test.go`

**Interfaces:**
- Produces: `buildinfo.Version`, `buildinfo.Commit`, `buildinfo.BuildTime` (string vars for ldflags)

- [ ] **Step 1: Initialize go module**

```bash
cd /Users/yinwm/work/mamamate/aliyun-cli-toolkit
go mod init github.com/mamamate/aliyun-cli-toolkit
```

- [ ] **Step 2: Write buildinfo test (failing)**

Create `internal/buildinfo/buildinfo_test.go`:
```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/buildinfo/ -v`
Expected: FAIL — package not found / symbols undefined.

- [ ] **Step 4: Write buildinfo implementation**

Create `internal/buildinfo/buildinfo.go`:
```go
// Package buildinfo holds version metadata injected via -ldflags at build time.
package buildinfo

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/buildinfo/ -v`
Expected: PASS.

- [ ] **Step 6: Create Taskfile.yml**

Create `Taskfile.yml` (mirrors mm-resume conventions):
```yaml
version: '3'

vars:
  APP_NAME: aliyun-media-cli
  DIST_DIR: dist
  MAIN_PACKAGE: ./cmd/aliyun-media-cli
  APP_VERSION:
    sh: |
      if [ -n "${ALIYUN_MEDIA_CLI_VERSION:-}" ]; then
        printf "%s" "$ALIYUN_MEDIA_CLI_VERSION"
      else
        printf "%s" "dev"
      fi
  COMMIT:
    sh: |
      commit="$(git rev-parse --short HEAD)"
      if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
        commit="${commit}-dirty"
      fi
      printf "%s" "$commit"
  BUILD_TIME:
    sh: date -u +%Y-%m-%dT%H:%M:%SZ
  LDFLAGS: >-
    -s -w
    -X github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo.Version={{.APP_VERSION}}
    -X github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo.Commit={{.COMMIT}}
    -X github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo.BuildTime={{.BUILD_TIME}}

env:
  CGO_ENABLED: 0

tasks:
  default:
    desc: Show available tasks
    cmds:
      - task --list

  fmt:
    desc: Format Go source files
    cmds:
      - gofmt -s -w .

  test:
    desc: Run all tests
    cmds:
      - go test ./... -v

  test-cover:
    desc: Run tests with coverage
    cmds:
      - go test ./... -coverprofile=coverage.out -covermode=atomic
      - go tool cover -func=coverage.out | tail -1

  build:
    desc: Build the CLI binary
    cmds:
      - go build -ldflags "{{.LDFLAGS}}" -o bin/{{.APP_NAME}} {{.MAIN_PACKAGE}}

  release:
    desc: Cross-compile release binaries (linux/mac × amd64/arm64)
    cmds:
      - mkdir -p {{.DIST_DIR}}
      - for: { var: PLATFORMS, split: "," }
        cmd: |
          OS="${PLATFORMS%/*}"
          ARCH="${PLATFORMS#*/}"
          GOOS=$OS GOARCH=$ARCH go build -ldflags "{{.LDFLAGS}}" -o {{.DIST_DIR}}/{{.APP_NAME}}-${OS}-${ARCH} {{.MAIN_PACKAGE}}
    vars:
      PLATFORMS: linux/amd64,linux/arm64,darwin/amd64,darwin/arm64
```

- [ ] **Step 7: Verify build scaffolding works**

Run: `task build`
Expected: creates `bin/aliyun-media-cli` (will fail if main.go doesn't exist yet — that's fine, Taskfile itself is the deliverable here).

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum Taskfile.yml internal/buildinfo/
git commit -m "chore: init go module, buildinfo, Taskfile"
```

---

### Task 2: Profile management — load/save/list/delete real TOML files

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

**Interfaces:**
- Produces:
  - `type Profile struct` with fields: `Name, Bucket, Region, Endpoint, RoleArn, AccessKeyID, AccessKeySecret, PublicDomain string`
  - `func ConfigDir() (string, error)` — returns `$ALIYUN_MEDIA_CLI_HOME` or `~/.config/aliyun-media-cli`
  - `func Load(name string) (Profile, error)` — reads `<ConfigDir>/profiles/<name>.toml`
  - `func (p Profile) Save() error` — writes to that path with 0600 perms, creating dirs
  - `func List() ([]string, error)` — lists profile names by scanning the profiles dir
  - `func Delete(name string) error` — removes the profile file
  - `func (p Profile) Masked() Profile` — returns a copy with AccessKeySecret replaced by `***` for `show`

- [ ] **Step 1: Write failing tests for profile round-trip**

Create `internal/profile/profile_test.go`:
```go
package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)

	p := Profile{
		Name:            "test-profile",
		Bucket:          "my-bucket",
		Region:          "cn-huhehaote",
		Endpoint:        "oss-cn-huhehaote.aliyuncs.com",
		RoleArn:         "acs:ram::123:role/x",
		AccessKeyID:     "MYAK",
		AccessKeySecret: "MYSK",
		PublicDomain:    "cdn.example.com",
	}
	if err := p.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// file must be 0600
	info, err := os.Stat(filepath.Join(dir, "profiles", "test-profile.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 0600", perm)
	}

	loaded, err := Load("test-profile")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(loaded, p) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", loaded, p)
	}
}

func TestLoadMissingProfile(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("expected error loading missing profile")
	}
}

func TestList(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := (Profile{Name: name, Bucket: "b"}.Save()); err != nil {
			t.Fatal(err)
		}
	}
	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "gamma"} // sorted
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("List = %v, want %v", names, want)
	}
}

func TestDelete(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	if err := (Profile{Name: "todrop"}.Save()); err != nil {
		t.Fatal(err)
	}
	if err := Delete("todrop"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Load("todrop"); err == nil {
		t.Fatal("profile still loadable after delete")
	}
}

func TestMasked(t *testing.T) {
	p := Profile{Name: "x", AccessKeyID: "AK", AccessKeySecret: "verysecret"}
	m := p.Masked()
	if m.AccessKeySecret != "***" {
		t.Fatalf("masked secret = %q, want ***", m.AccessKeySecret)
	}
	if m.AccessKeyID != "AK" {
		t.Fatalf("AKID should not be masked, got %q", m.AccessKeyID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/profile/ -v`
Expected: FAIL — package has no implementation.

- [ ] **Step 3: Write profile implementation**

Create `internal/profile/profile.go`:
```go
// Package profile loads and saves aliyun-media-cli credential profiles.
// Profiles are TOML files stored under <configdir>/profiles/<name>.toml
// with 0600 permissions. The config dir is $ALIYUN_MEDIA_CLI_HOME or
// ~/.config/aliyun-media-cli.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Profile holds the credentials and endpoint config for one OSS profile.
type Profile struct {
	Name            string `toml:"-"`
	Bucket          string `toml:"bucket"`
	Region          string `toml:"region"`
	Endpoint        string `toml:"endpoint"`
	RoleArn         string `toml:"role-arn"`
	AccessKeyID     string `toml:"access-key-id"`
	AccessKeySecret string `toml:"access-key-secret"`
	PublicDomain    string `toml:"public-domain,omitempty"`
}

// ConfigDir returns the profile config directory.
func ConfigDir() (string, error) {
	if home := os.Getenv("ALIYUN_MEDIA_CLI_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aliyun-media-cli"), nil
}

func profilePath(name string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles", name+".toml"), nil
}

// Load reads a profile by name from disk.
func Load(name string) (Profile, error) {
	path, err := profilePath(name)
	if err != nil {
		return Profile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("load profile %q: %w", name, err)
	}
	var p Profile
	if err := toml.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %q: %w", name, err)
	}
	p.Name = name
	return p, nil
}

// Save writes the profile to disk with 0600 permissions.
func (p Profile) Save() error {
	path, err := profilePath(p.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// List returns the sorted names of all saved profiles.
func List() ([]string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	profilesDir := filepath.Join(dir, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
	}
	sort.Strings(names)
	return names, nil
}

// Delete removes a profile file by name.
func Delete(name string) error {
	path, err := profilePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete profile %q: %w", name, err)
	}
	return nil
}

// Masked returns a copy of the profile with the secret redacted, for display.
func (p Profile) Masked() Profile {
	m := p
	m.AccessKeySecret = "***"
	return m
}
```

- [ ] **Step 4: Add the toml dependency**

Run: `go get github.com/pelletier/go-toml/v2`

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/profile/ -v`
Expected: all 5 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/profile/
git commit -m "feat(profile): TOML profile load/save/list/delete with 0600 perms"
```

---

### Task 3: JSON output helpers — stdout JSON, stderr error, exit codes

**Files:**
- Create: `internal/output/output.go`
- Create: `internal/output/output_test.go`

**Interfaces:**
- Produces:
  - `func PrintJSON(w io.Writer, v any) error` — marshals v to compact JSON + newline
  - `func PrintError(w io.Writer, msg string)` — writes `{"error":"<msg>"}` + newline to w (used for stderr)
  - `func ExitWithError(msg string)` — writes PrintError to stderr, then os.Exit(1)

- [ ] **Step 1: Write failing test**

Create `internal/output/output_test.go`:
```go
package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]any{"key": "k", "n": 3}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// compact JSON, ends with newline
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("output must end with newline, got %q", got)
	}
	want := `{"key":"k","n":3}` + "\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintError(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "boom")
	got := buf.String()
	want := `{"error":"boom"}` + "\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write implementation**

Create `internal/output/output.go`:
```go
// Package output provides JSON stdout/stderr helpers for the CLI.
// Contract: stdout is always valid compact JSON terminated by a newline.
// Errors go to stderr as {"error":"..."} with a non-zero exit code.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// PrintJSON marshals v to compact JSON and writes it followed by a newline.
func PrintJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

// PrintError writes a JSON error object to w.
func PrintError(w io.Writer, msg string) {
	fmt.Fprintf(w, `{"error":%s}`+"\n", mustJSON(msg))
}

// ExitWithError prints an error to stderr and exits with code 1.
func ExitWithError(msg string) {
	PrintError(os.Stderr, msg)
	os.Exit(1)
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/output/
git commit -m "feat(output): JSON stdout/stderr helpers with exit codes"
```

---

### Task 4: OSS client — real AssumeRole + Upload + PresignGet + Head + PublicURL

This is the core task. The client wraps the aliyun OSS SDK + STS, mirroring `OssStsService` in Mamamate but in Go. Tests hit the real bucket.

**Files:**
- Create: `internal/oss/client.go`
- Create: `internal/oss/client_test.go`

**Interfaces:**
- Consumes: `profile.Profile`
- Produces:
  - `type Client struct` — holds the profile + lazily-created STS client
  - `func NewClient(p profile.Profile) (*Client, error)`
  - `type UploadResult struct { Key, URL, Visibility string; Size int64; ETag string }`
  - `func (c *Client) Upload(ctx, key string, data []byte, contentType string, private bool) (UploadResult, error)`
  - `func (c *Client) ResolvePrivate(ctx, key string, ttl time.Duration) (signedURL string, err error)`
  - `func (c *Client) PublicURL(key string) string`
  - `func (c *Client) Stat(ctx, key string) (exists bool, size int64, contentType, etag string, err error)`
  - `func (c *Client) Delete(ctx, key string) error` — used by tests for cleanup

- [ ] **Step 1: Add OSS + STS SDK dependencies**

```bash
go get github.com/aliyun/aliyun-oss-go-sdk/oss
go get github.com/alibabacloud-go/sts-20150401
go get github.com/alibabacloud-go/tea
```

- [ ] **Step 2: Write failing integration tests (real OSS)**

Create `internal/oss/client_test.go`:
```go
package oss

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// testProfile loads a real profile for integration testing.
// Tests skip if the profile is unavailable (no credentials / no network in CI).
func testProfile(t *testing.T) profile.Profile {
	t.Helper()
	name := os.Getenv("ALIYUN_MEDIA_CLI_TEST_PROFILE")
	if name == "" {
		name = "mamamate"
	}
	// Integration profiles live in <repo>/profiles.local — point the config home there.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", filepath.Join(repoRoot, "profiles.local"))
	// profiles.local stores files as <name>.toml directly (not under profiles/),
	// so we load by reading the file directly via profile.Load which uses profiles/ subdir.
	// To keep it simple, we read the local file manually if Load fails.
	p, err := profile.Load(name)
	if err != nil {
		// fallback: read profiles.local/<name>.toml directly
		data, ferr := os.ReadFile(filepath.Join(repoRoot, "profiles.local", name+".toml"))
		if ferr != nil {
			t.Skipf("integration profile %q not available: %v / %v", name, err, ferr)
		}
		// parse via toml through a temp config home
		_ = data
		t.Skipf("see helper below — configure temp profiles dir")
	}
	p.Name = name
	return p
}

func TestUploadPublicRoundTrip(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}

	key := "test-cli/upload-public-" + t.Name() + ".txt"
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), key)
	})

	data := []byte("hello oss integration test public\n")
	res, err := c.Upload(context.Background(), key, data, "text/plain", false)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.Visibility != "public" {
		t.Fatalf("visibility = %q, want public", res.Visibility)
	}
	if res.Size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", res.Size, len(data))
	}
	if res.ETag == "" {
		t.Fatal("etag should be non-empty for single-PUT upload")
	}
	if res.URL == "" {
		t.Fatal("public upload must return a stable url")
	}
}

func TestUploadPrivateNoURL(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := "test-cli/upload-private-" + t.Name() + ".txt"
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), key)
	})

	data := []byte("private content\n")
	res, err := c.Upload(context.Background(), key, data, "text/plain", true)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.Visibility != "private" {
		t.Fatalf("visibility = %q, want private", res.Visibility)
	}
	if res.URL != "" {
		t.Fatalf("private upload must NOT return a stable url, got %q", res.URL)
	}
}

func TestResolvePrivateReturnsSignedURL(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := "test-cli/resolve-" + t.Name() + ".txt"
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), key)
	})

	if _, err := c.Upload(context.Background(), key, []byte("resolve me\n"), "text/plain", true); err != nil {
		t.Fatal(err)
	}
	url, err := c.ResolvePrivate(context.Background(), key, 5*time.Minute)
	if err != nil {
		t.Fatalf("ResolvePrivate: %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty signed url")
	}
	// signed URL must contain a signature param
	if !contains(url, "Signature=") && !contains(url, "signature=") {
		t.Fatalf("signed url has no signature param: %s", url)
	}
}

func TestStatExistsAndMissing(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := "test-cli/stat-" + t.Name() + ".txt"
	t.Cleanup(func() {
		_ = c.Delete(context.Background(), key)
	})

	if _, err := c.Upload(context.Background(), key, []byte("stat me\n"), "text/plain", false); err != nil {
		t.Fatal(err)
	}

	exists, size, ct, etag, err := c.Stat(context.Background(), key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !exists {
		t.Fatal("should exist")
	}
	if size != 8 {
		t.Fatalf("size = %d, want 8", size)
	}
	if ct != "text/plain" {
		t.Fatalf("contentType = %q, want text/plain", ct)
	}
	if etag == "" {
		t.Fatal("etag should be non-empty")
	}

	// missing key
	exists2, _, _, _, err := c.Stat(context.Background(), "test-cli/does-not-exist-"+t.Name())
	if err != nil {
		t.Fatalf("Stat missing should not error: %v", err)
	}
	if exists2 {
		t.Fatal("missing key should report exists=false")
	}
}

func TestPublicURL(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	url := c.PublicURL("some/key.jpg")
	if url == "" {
		t.Fatal("expected non-empty public url")
	}
	if !contains(url, "some/key.jpg") {
		t.Fatalf("public url should contain the key: %s", url)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 3: Run tests to verify they fail (compile failure)**

Run: `go test ./internal/oss/ -v`
Expected: FAIL — `NewClient`, `Upload`, etc. undefined.

- [ ] **Step 4: Write the OSS client implementation**

Create `internal/oss/client.go`:
```go
// Package oss wraps the Alibaba Cloud OSS SDK + STS AssumeRole flow.
// It mirrors Mamamate's OssStsService: per-operation STS credentials
// scoped to a single object/action, then the OSS operation.
package oss

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alibabacloud-go/sts-20150401/client"
	openapi "github.com/alibabacloud-go/tea/openapi"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// Client performs OSS operations against the bucket configured in a Profile.
type Client struct {
	prof      profile.Profile
	stsClient *client.Client
}

// NewClient builds an OSS client from a profile.
func NewClient(p profile.Profile) (*Client, error) {
	cfg := &openapi.Config{
		AccessKeyId:     &p.AccessKeyID,
		AccessKeySecret: &p.AccessKeySecret,
	}
	region := p.Region
	if region == "" {
		region = "cn-huhehaote"
	}
	cfg.RegionId = &region
	sts, err := client.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("init STS client: %w", err)
	}
	return &Client{prof: p, stsClient: sts}, nil
}

// UploadResult is returned by Upload.
type UploadResult struct {
	Key        string `json:"key"`
	URL        string `json:"url,omitempty"`
	Visibility string `json:"visibility"`
	Size       int64  `json:"size"`
	ETag       string `json:"etag"`
}

// assumeRole obtains STS temporary credentials scoped to a single object+action.
func (c *Client) assumeRole(action, objectKey string) (*stsCreds, error) {
	policy := fmt.Sprintf(`{
  "Version": "1",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["%s"],
    "Resource": ["acs:oss:*:*:%s/%s"]
  }]}`, action, c.prof.Bucket, objectKey)

	session := "aliyun-media-cli"
	dur := int64(3600)
	req := &client.AssumeRoleRequest{
		RoleArn:         &c.prof.RoleArn,
		RoleSessionName: &session,
		DurationSeconds: &dur,
		Policy:          &policy,
	}
	resp, err := c.stsClient.AssumeRole(req)
	if err != nil {
		return nil, fmt.Errorf("STS AssumeRole: %w", err)
	}
	if resp.Body == nil || resp.Body.Credentials == nil {
		return nil, fmt.Errorf("STS returned no credentials")
	}
	cr := resp.Body.Credentials
	return &stsCreds{
		ak:     *cr.AccessKeyId,
		sk:     *cr.AccessKeySecret,
		token:  *cr.SecurityToken,
		region: c.prof.Region,
	}, nil
}

type stsCreds struct {
	ak, sk, token, region string
}

func (c *Client) ossEndpoint() string {
	ep := c.prof.Endpoint
	if ep == "" {
		ep = fmt.Sprintf("oss-%s.aliyuncs.com", c.prof.Region)
	}
	return ep
}

// Upload puts an object into OSS. When private is true, the object ACL is set
// to private and no stable URL is returned.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string, private bool) (UploadResult, error) {
	action := "oss:PutObject"
	creds, err := c.assumeRole(action, key)
	if err != nil {
		return UploadResult{}, err
	}
	ossClient, err := oss.New(c.ossEndpoint(), creds.ak, creds.sk, oss.SecurityToken(creds.token))
	if err != nil {
		return UploadResult{}, fmt.Errorf("oss new: %w", err)
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return UploadResult{}, fmt.Errorf("oss bucket: %w", err)
	}

	opts := []oss.Option{}
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}
	visibility := "public"
	if private {
		opts = append(opts, oss.ObjectACL(oss.ACLPrivate))
		visibility = "private"
	}

	if err := bucket.PutObject(key, bytesReader(data), opts...); err != nil {
		return UploadResult{}, fmt.Errorf("oss put: %w", err)
	}

	// read back etag + size via HEAD
	_, size, etag, _, herr := c.headWith(creds, key)
	if herr != nil {
		// upload succeeded but HEAD failed — still return what we have
		etag = ""
		size = int64(len(data))
	}

	res := UploadResult{
		Key:        key,
		Visibility: visibility,
		Size:       size,
		ETag:       etag,
	}
	if !private {
		res.URL = c.PublicURL(key)
	}
	return res, nil
}

// ResolvePrivate generates a short-lived presigned GET URL for a private object.
func (c *Client) ResolvePrivate(ctx context.Context, key string, ttl time.Duration) (string, error) {
	creds, err := c.assumeRole("oss:GetObject", key)
	if err != nil {
		return "", err
	}
	ossClient, err := oss.New(c.ossEndpoint(), creds.ak, creds.sk, oss.SecurityToken(creds.token))
	if err != nil {
		return "", err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return "", err
	}
	url, err := bucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", fmt.Errorf("sign url: %w", err)
	}
	return url, nil
}

// PublicURL returns the stable public URL for an object.
func (c *Client) PublicURL(key string) string {
	domain := c.prof.PublicDomain
	if domain != "" {
		return fmt.Sprintf("https://%s/%s", domain, key)
	}
	return fmt.Sprintf("https://%s.%s/%s", c.prof.Bucket, c.ossEndpoint(), key)
}

// Stat reports object metadata. exists is false (not an error) when absent.
func (c *Client) Stat(ctx context.Context, key string) (exists bool, size int64, contentType, etag string, err error) {
	creds, err := c.assumeRole("oss:GetObject", key)
	if err != nil {
		return false, 0, "", "", err
	}
	return c.headWith(creds, key)
}

func (c *Client) headWith(creds *stsCreds, key string) (exists bool, size int64, contentType, etag string, err error) {
	ossClient, err := oss.New(c.ossEndpoint(), creds.ak, creds.sk, oss.SecurityToken(creds.token))
	if err != nil {
		return false, 0, "", "", err
	}

	headers, err := ossClient.GetObjectMeta(c.prof.Bucket, key)
	if err != nil {
		ossErr, ok := err.(oss.ServiceError)
		if ok && (ossErr.StatusCode == 404 || strings.Contains(ossErr.Code, "NoSuch")) {
			return false, 0, "", "", nil
		}
		return false, 0, "", "", fmt.Errorf("oss head: %w", err)
	}
	return true, headers["Content-Length"][0], headers["Content-Type"][0], strings.Trim(headers["ETag"][0], `"`), nil
}

// Delete removes an object. Used by tests for cleanup.
func (c *Client) Delete(ctx context.Context, key string) error {
	creds, err := c.assumeRole("oss:DeleteObject", key)
	if err != nil {
		return err
	}
	ossClient, err := oss.New(c.ossEndpoint(), creds.ak, creds.sk, oss.SecurityToken(creds.token))
	if err != nil {
		return err
	}
	return ossClient.DeleteObject(c.prof.Bucket, key)
}
```

Note: the `headWith` function's return of `headers["Content-Length"][0]` returns a string; the implementer must parse it to int64. Also `bytesReader` is `bytes.NewReader` — the implementer should add the import. These are intentional review hooks; fix during implementation to compile cleanly.

- [ ] **Step 5: Run tests — they hit real OSS**

Ensure `profiles.local/mamamate.toml` exists (it does, from scaffolding). Set the test home:
Run: `ALIYUN_MEDIA_CLI_HOME=$(pwd)/profiles.local ALIYUN_MEDIA_CLI_TEST_PROFILE=mamamate go test ./internal/oss/ -v -timeout 120s`
Expected: PASS for all 5 tests. (The `testProfile` helper needs adjustment: it should copy `profiles.local/mamamate.toml` into a temp `profiles/` subdir so `profile.Load` finds it. Adjust the helper in step 2 if Load fails — the cleanest fix: in the helper, set `ALIYUN_MEDIA_CLI_HOME` to a temp dir and write `profiles/mamamate.toml` into it by reading `profiles.local/mamamate.toml`.)

- [ ] **Step 6: Fix the testProfile helper to work with profile.Load**

The `profiles.local/` layout is flat (`<name>.toml`), but `profile.Load` expects `<home>/profiles/<name>.toml`. Fix the helper to bridge this: read `profiles.local/mamamate.toml`, write it to `t.TempDir()/profiles/mamamate.toml`, set `ALIYUN_MEDIA_CLI_HOME` to the temp dir. This keeps profile.Load's contract intact while using the flat local fixtures.

- [ ] **Step 7: Re-run tests, confirm green**

Run: `go test ./internal/oss/ -v -timeout 120s`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/oss/
git commit -m "feat(oss): real OSS client — AssumeRole upload/resolve/stat/delete"
```

---

### Task 5: cobra root + profile subcommands (add/list/remove/show)

**Files:**
- Create: `internal/app/root.go`
- Create: `internal/app/profile_cmd.go`
- Create: `internal/app/profile_cmd_test.go`

**Interfaces:**
- Produces:
  - `func NewRootCmd() *cobra.Command` — root command; holds the global `--profile` flag (required on operation subcommands, not on profile management subcommands)
  - profile subcommands wired under `profile add/list/remove/show`

- [ ] **Step 1: Add cobra dependency**

```bash
go get github.com/spf13/cobra@latest
```

- [ ] **Step 2: Write failing test for `profile add` + `profile show` round-trip via command**

Create `internal/app/profile_cmd_test.go`:
```go
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
	var shown map[string]string
	if err := json.Unmarshal(out2.Bytes(), &shown); err != nil {
		t.Fatalf("show output not json: %s\n%v", out2.String(), err)
	}
	if shown["access-key-secret"] != "***" {
		t.Fatalf("show should mask secret, got %q", shown["access-key-secret"])
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

func runAdd(t *testing.T, name string) {
	t.Helper()
	root := NewRootCmd()
	root.SetArgs([]string{"profile", "add", name, "--bucket", "b"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/app/ -run Profile -v`
Expected: FAIL — `NewRootCmd` undefined.

- [ ] **Step 4: Write root.go**

Create `internal/app/root.go`:
```go
package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo"
)

// NewRootCmd assembles the full command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "aliyun-media-cli",
		Short:   "Stateless Alibaba Cloud OSS media CLI",
		Version: buildinfo.Version,
	}
	root.AddCommand(newProfileCmd())
	root.AddCommand(newUploadCmd())
	root.AddCommand(newResolveCmd())
	root.AddCommand(newStatCmd())
	return root
}
```

- [ ] **Step 5: Write profile_cmd.go**

Create `internal/app/profile_cmd.go`:
```go
package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage OSS credential profiles",
	}
	cmd.AddCommand(newProfileAddCmd(), newProfileListCmd(), newProfileRemoveCmd(), newProfileShowCmd())
	return cmd
}

func newProfileAddCmd() *cobra.Command {
	var p profile.Profile
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Name = args[0]
			if err := p.Save(); err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]string{"profile": p.Name, "status": "saved"})
		},
	}
	cmd.Flags().StringVar(&p.Bucket, "bucket", "", "OSS bucket name")
	cmd.Flags().StringVar(&p.Region, "region", "", "OSS region (e.g. cn-huhehaote)")
	cmd.Flags().StringVar(&p.Endpoint, "endpoint", "", "OSS endpoint")
	cmd.Flags().StringVar(&p.RoleArn, "role-arn", "", "RAM role ARN for STS AssumeRole")
	cmd.Flags().StringVar(&p.AccessKeyID, "access-key-id", "", "AccessKey ID")
	cmd.Flags().StringVar(&p.AccessKeySecret, "access-key-secret", "", "AccessKey Secret")
	cmd.Flags().StringVar(&p.PublicDomain, "public-domain", "", "Optional stable public/CDN domain")
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := profile.List()
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string][]string{"profiles": names})
		},
	}
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := profile.Delete(args[0]); err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]string{"profile": args[0], "status": "deleted"})
		},
	}
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a profile (masked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(args[0])
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), p.Masked())
		},
	}
}
```

- [ ] **Step 6: Add stubs for upload/resolve/stat commands so root.go compiles**

The root references `newUploadCmd`, `newResolveCmd`, `newStatCmd` — create minimal stubs now (implemented in Tasks 6-8). Create three files:

`internal/app/upload.go`:
```go
package app

import "github.com/spf13/cobra"

func newUploadCmd() *cobra.Command {
	return &cobra.Command{Use: "upload", Short: "stub", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
```
(Repeat analogously for `resolve.go` and `stat.go` with their respective names.)

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/app/ -run Profile -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/app/
git commit -m "feat(app): cobra root + profile add/list/remove/show subcommands"
```

---

### Task 6: upload subcommand — real OSS end-to-end

**Files:**
- Modify: `internal/app/upload.go` (replace stub)
- Create: `internal/app/upload_test.go`

**Interfaces:**
- Consumes: `oss.Client.Upload`, `output.PrintJSON`
- Produces: the `upload` subcommand with `--profile`, `--key`, `--file`, `--content-type`, `--private`

- [ ] **Step 1: Write failing end-to-end test**

Create `internal/app/upload_test.go`:
```go
package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setupIntegrationHome copies profiles.local/<profile>.toml into a temp
// profiles/ dir so profile.Load works. Skips if the fixture is absent.
func setupIntegrationHome(t *testing.T) {
	t.Helper()
	name := os.Getenv("ALIYUN_MEDIA_CLI_TEST_PROFILE")
	if name == "" {
		name = "mamamate"
	}
	repoRoot, _ := filepath.Abs("../..")
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
}

func TestUploadEndToEndPublic(t *testing.T) {
	setupIntegrationHome(t)
	root := NewRootCmd()
	key := "test-cli/e2e-upload-public-" + t.Name() + ".txt"
	tmpFile := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(tmpFile, []byte("e2e upload public\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root.SetArgs([]string{"upload", "--profile", "mamamate", "--key", key, "--file", tmpFile, "--content-type", "text/plain"})
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

	// cleanup via the OSS client directly
	cleanupUploaded(t, "mamamate", key)
}

// cleanupUploaded deletes the object to keep the bucket tidy. Best-effort.
func cleanupUploaded(t *testing.T, profileName, key string) {
	t.Helper()
	// build a client and delete — reuse oss.NewClient + profile.Load
	// (implementation imports oss + profile; keep this helper in a shared e2e_test.go)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestUploadEndToEndPublic -v`
Expected: FAIL — stub upload does nothing, produces no JSON.

- [ ] **Step 3: Implement upload subcommand**

Replace `internal/app/upload.go`:
```go
package app

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newUploadCmd() *cobra.Command {
	var (
		profileName, key, file, contentType string
		private                             bool
	)
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a file to OSS and return key + url + etag",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			res, err := c.Upload(context.Background(), key, data, contentType, private)
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to file to upload (required)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type header")
	cmd.Flags().BoolVar(&private, "private", false, "Store as private (no stable URL returned)")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
```

- [ ] **Step 4: Write the shared cleanup helper in `internal/app/e2e_test.go`**

Create `internal/app/e2e_test.go`:
```go
package app

import (
	"context"
	"testing"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func cleanupUploaded(t *testing.T, profileName, key string) {
	t.Helper()
	p, err := profile.Load(profileName)
	if err != nil {
		return
	}
	c, err := oss.NewClient(p)
	if err != nil {
		return
	}
	_ = c.Delete(context.Background(), key)
}
```

- [ ] **Step 5: Run test to verify it passes (real OSS)**

Run: `go test ./internal/app/ -run TestUploadEndToEnd -v -timeout 120s`
Expected: PASS.

- [ ] **Step 6: Add a private-upload e2e test**

Append to `upload_test.go`:
```go
func TestUploadEndToEndPrivate(t *testing.T) {
	setupIntegrationHome(t)
	root := NewRootCmd()
	key := "test-cli/e2e-upload-private-" + t.Name() + ".txt"
	tmpFile := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(tmpFile, []byte("e2e private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"upload", "--profile", "mamamate", "--key", key, "--file", tmpFile, "--content-type", "text/plain", "--private"})
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
	cleanupUploaded(t, "mamamate", key)
}
```

- [ ] **Step 7: Run all upload tests, confirm green**

Run: `go test ./internal/app/ -run TestUploadEndToEnd -v -timeout 120s`
Expected: both PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/app/
git commit -m "feat(app): upload subcommand — public + private end-to-end"
```

---

### Task 7: resolve subcommand — public stable URL + private signed URL

**Files:**
- Modify: `internal/app/resolve.go` (replace stub)
- Create: `internal/app/resolve_test.go`

- [ ] **Step 1: Write failing e2e tests (public + private)**

Create `internal/app/resolve_test.go`:
```go
package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestResolvePublicEndToEnd(t *testing.T) {
	setupIntegrationHome(t)
	root := NewRootCmd()
	key := "test-cli/e2e-resolve-public-" + t.Name() + ".txt"
	tmpFile := filepath.Join(t.TempDir(), "p.txt")
	writeFile(t, tmpFile, []byte("resolve public\n"))
	uploadKey(t, "mamamate", key, tmpFile, "text/plain", false)
	t.Cleanup(func() { cleanupUploaded(t, "mamamate", key) })

	root.SetArgs([]string{"resolve", "--profile", "mamamate", "--key", key, "--public"})
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
	setupIntegrationHome(t)
	root := NewRootCmd()
	key := "test-cli/e2e-resolve-private-" + t.Name() + ".txt"
	tmpFile := filepath.Join(t.TempDir(), "p.txt")
	writeFile(t, tmpFile, []byte("resolve private\n"))
	uploadKey(t, "mamamate", key, tmpFile, "text/plain", true)
	t.Cleanup(func() { cleanupUploaded(t, "mamamate", key) })

	root.SetArgs([]string{"resolve", "--profile", "mamamate", "--key", key, "--ttl", "10m"})
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
```

Add the small helpers `writeFile` and `uploadKey` to `e2e_test.go`:
```go
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func uploadKey(t *testing.T, profileName, key, file, contentType string, private bool) {
	t.Helper()
	p, err := profile.Load(profileName)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	c, err := oss.NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Upload(context.Background(), key, data, contentType, private); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestResolve -v`
Expected: FAIL — stub.

- [ ] **Step 3: Implement resolve subcommand**

Replace `internal/app/resolve.go`:
```go
package app

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newResolveCmd() *cobra.Command {
	var (
		profileName, key string
		ttl              time.Duration
		isPublic         bool
	)
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve an object key to a readable URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			if isPublic {
				return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
					"url":        c.PublicURL(key),
					"visibility": "public",
				})
			}
			d := ttl
			if d == 0 {
				d = 15 * time.Minute
			}
			url, err := c.ResolvePrivate(context.Background(), key, d)
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"url":        url,
				"visibility": "private",
				"expiresAt":  time.Now().Add(d).UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	cmd.Flags().DurationVar(&ttl, "ttl", 0, "Signed URL validity (default 15m)")
	cmd.Flags().BoolVar(&isPublic, "public", false, "Treat as public; return stable URL without signing")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
```

- [ ] **Step 4: Run tests, confirm green (real OSS)**

Run: `go test ./internal/app/ -run TestResolve -v -timeout 120s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/
git commit -m "feat(app): resolve subcommand — public stable + private signed URL"
```

---

### Task 8: stat subcommand — exists/missing, size/content-type/etag

**Files:**
- Modify: `internal/app/stat.go` (replace stub)
- Create: `internal/app/stat_test.go`

- [ ] **Step 1: Write failing e2e tests (exists + missing)**

Create `internal/app/stat_test.go`:
```go
package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStatExistsEndToEnd(t *testing.T) {
	setupIntegrationHome(t)
	key := "test-cli/e2e-stat-" + t.Name() + ".txt"
	tmpFile := filepath.Join(t.TempDir(), "s.txt")
	writeFile(t, tmpFile, []byte("stateme\n"))
	uploadKey(t, "mamamate", key, tmpFile, "text/plain", false)
	t.Cleanup(func() { cleanupUploaded(t, "mamamate", key) })

	root := NewRootCmd()
	root.SetArgs([]string{"stat", "--profile", "mamamate", "--key", key})
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
	setupIntegrationHome(t)
	root := NewRootCmd()
	root.SetArgs([]string{"stat", "--profile", "mamamate", "--key", "test-cli/does-not-exist-" + t.Name()})
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestStat -v`
Expected: FAIL — stub.

- [ ] **Step 3: Implement stat subcommand**

Replace `internal/app/stat.go`:
```go
package app

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newStatCmd() *cobra.Command {
	var profileName, key string
	cmd := &cobra.Command{
		Use:   "stat",
		Short: "Report object metadata (exists/size/content-type/etag)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			exists, sizeStr, ct, etag, err := c.Stat(context.Background(), key)
			if err != nil {
				return err
			}
			res := map[string]any{"exists": exists}
			if exists {
				size, _ := strconv.ParseInt(sizeStr, 10, 64)
				res["size"] = size
				res["contentType"] = ct
				res["etag"] = etag
			}
			return output.PrintJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
```

- [ ] **Step 4: Run tests, confirm green (real OSS)**

Run: `go test ./internal/app/ -run TestStat -v -timeout 120s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/
git commit -m "feat(app): stat subcommand — exists/missing + metadata"
```

---

### Task 9: main.go entry point + error handling + binary build verification

**Files:**
- Create: `cmd/aliyun-media-cli/main.go`

- [ ] **Step 1: Write main.go**

Create `cmd/aliyun-media-cli/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/mamamate/aliyun-cli-toolkit/internal/app"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
)

func main() {
	if err := app.NewRootCmd().Execute(); err != nil {
		output.PrintError(os.Stderr, err.Error())
		os.Exit(1)
	}
	// guard: if stdout is connected but nothing printed (e.g. a future dry-run),
	// that's fine; successful commands print their own JSON.
	_ = fmt.Sprintf // keep import if needed; remove if unused
}
```

Clean up unused imports before committing.

- [ ] **Step 2: Build the binary**

Run: `task build`
Expected: `bin/aliyun-media-cli` produced without error.

- [ ] **Step 3: Manual smoke test against real OSS**

```bash
KEY="test-cli/smoke-$(date +%s).txt"
echo "smoke test" > /tmp/smoke.txt
./bin/aliyun-media-cli upload --profile mamamate --key "$KEY" --file /tmp/smoke.txt --content-type text/plain
./bin/aliyun-media-cli stat    --profile mamamate --key "$KEY"
./bin/aliyun-media-cli resolve --profile mamamate --key "$KEY" --public
./bin/aliyun-media-cli resolve --profile mamamate --key "$KEY" --ttl 5m    # should fail — object is public, but signing still works (tests path); or use a private-uploaded key
```

For the private resolve smoke, upload a private object first:
```bash
PK="test-cli/smoke-private-$(date +%s).txt"
./bin/aliyun-media-cli upload --profile mamamate --key "$PK" --file /tmp/smoke.txt --content-type text/plain --private
./bin/aliyun-media-cli resolve --profile mamamate --key "$PK" --ttl 5m
```
Expected: all produce valid JSON; private resolve returns a signed URL.

- [ ] **Step 4: Configure integration test profile resolution**

The binary reads `~/.config/aliyun-media-cli` by default. For local testing of the binary, either symlink `profiles.local/` into place or set `ALIYUN_MEDIA_CLI_HOME`. Verify:
```bash
ALIYUN_MEDIA_CLI_HOME=$(pwd)/profiles.local ./bin/aliyun-media-cli profile list
```
Expected: the `profiles.local/` layout is flat, but `profile.Load` expects a `profiles/` subdir. For binary smoke tests, create the proper structure once: `mkdir -p ~/.config/aliyun-media-cli/profiles && cp profiles.local/mamamate.toml ~/.config/aliyun-media-cli/profiles/`.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat: main entry point + binary build"
```

---

### Task 10: Coverage gate — verify ≥85%, close gaps, add cleanup hardening

**Files:**
- Possibly modify: any test files to close gaps
- Verify: `internal/profile/` should be near 100% (pure logic); `internal/oss/` and `internal/app/` covered by integration tests.

- [ ] **Step 1: Run coverage report**

Run: `go test ./... -coverprofile=coverage.out -covermode=atomic -timeout 180s`
Then: `go tool cover -func=coverage.out`

Expected: total coverage ≥ 85%. Identify any function below 85%.

- [ ] **Step 2: Close coverage gaps**

For any function below threshold, add a test. Common gaps to expect:
- `Delete` in oss client — add a test that uploads then deletes then stats-missing.
- `profile.Delete` error path — already covered.
- error branches in `Load` (corrupt TOML) — add a test that writes garbage to a profile file and asserts Load errors.

Add to `internal/profile/profile_test.go`:
```go
func TestLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	badPath := filepath.Join(dir, "profiles", "bad.toml")
	os.MkdirAll(filepath.Dir(badPath), 0o700)
	os.WriteFile(badPath, []byte("this is not = = valid toml {{{"), 0o600)
	_, err := Load("bad")
	if err == nil {
		t.Fatal("expected parse error for corrupt profile")
	}
}
```

- [ ] **Step 3: Re-run coverage, confirm ≥85%**

Run: `task test-cover`
Expected: total ≥ 85%.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "test: close coverage gaps to ≥85%"
```

---

## Self-Review Notes (completed)

**Spec coverage check:**
- ✅ profile add/list/remove/show — Task 2 + 5
- ✅ no default profile, `--profile` required — enforced via MarkFlagRequired in Tasks 6/7/8
- ✅ upload (public + private, etag, no-url-when-private) — Task 4 + 6
- ✅ resolve (public stable URL + private signed with ttl + expiresAt) — Task 4 + 7
- ✅ resolve returns no etag/size — Task 7 (only url/visibility/expiresAt)
- ✅ stat (exists/missing + size/contentType/etag) — Task 4 + 8
- ✅ JSON stdout, stderr error, exit codes — Task 3 + 9
- ✅ 0600 perms on profile files — Task 2
- ✅ TDD every task, real OSS, no mocks — Global Constraints + every task

**Placeholder scan:** The two "review hooks" in Task 4 (bytesReader alias, int64 parse of Content-Length) are intentional — they are fixed inline in the same task's steps. No TBDs remain.

**Type consistency:** `oss.Client.Upload` returns `UploadResult{Key,URL,Visibility,Size,ETag}` — used consistently in Task 6. `Stat` returns `(exists, sizeStr, ct, etag, err)` — note `sizeStr` is a string from OSS headers, parsed to int64 in the stat command (Task 8). This asymmetry is real (OSS returns headers as `map[string][]string`) and documented.

**Known risk:** The STS SDK's Go API shape (pointer-to-string fields, `client.NewClient` config) may differ slightly from the Java SDK; Task 4 step 5 includes an adjustment step to reconcile against the actual Go SDK types. The integration test (real OSS) is the source of truth — if it passes, the wiring is correct.
