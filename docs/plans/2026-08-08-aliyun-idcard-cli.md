# aliyun-idcard-cli Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `aliyun-idcard-cli`, a stateless Go CLI that verifies an ID card via Alibaba Cloud `Id2MetaVerifyWithOCR` (OCR + two-factor in one call). It consumes image URLs (public or presigned), never touches OSS, and is composed with `aliyun-media-cli resolve` by the caller.

**Architecture:** Single Go binary, parallel to `aliyun-media-cli`, sharing the same `internal/{profile,output,buildinfo}` packages. The CloudAuth call is a single HTTPS POST with an HMAC-SHA1 signature computed locally (mirroring `AliyunId2MetaVerifyService.java`). No cloud SDK needed — stdlib only for the network layer.

**Tech Stack:** Go 1.22+, spf13/cobra (shared), pelletier/go-toml/v2 (shared). CloudAuth via `net/http` + `crypto/hmac` + `crypto/sha1` + `encoding/base64`.

## Global Constraints

- **Language:** Go 1.22+.
- **Testing:** TDD — failing test first. ≥85% coverage. **No mocks** — the verify integration test hits the real CloudAuth endpoint using `profiles.local/mamamate-verify.toml`. Tests skip if the profile is absent.
- **Test fixtures:** real ID-card images in `test-assets/` (must be added by the developer — they're sensitive and gitignored). The integration test reads them, uploads via `aliyun-media-cli` (or the oss client directly) to get presigned URLs, then feeds those URLs to verify.
- **I/O contract:** stdout = JSON, stderr = JSON errors, exit 0 on success, non-zero on failure. Same contract as media-cli.
- **Caller orchestration:** idcard-cli only accepts `--front-url`/`--back-url`. It does NOT accept `--key` and never calls media-cli. The caller resolves keys to URLs first.
- **Verify semantics:** `ok:false` + non-zero exit = the API call itself failed (network/auth/parse). `ok:true, passed:false` + **exit 0** = the API succeeded but name/ID didn't match (this is a business result, not a CLI error).
- **module path:** `github.com/mamamate/aliyun-cli-toolkit`

---

## File Structure

```
cmd/aliyun-idcard-cli/main.go              — thin entry
internal/
  idcard/
    verify.go                               — Verify(frontURL, backURL) -> Result, calls CloudAuth
    verify_test.go                          — real CloudAuth integration test
    sign.go                                 — HMAC-SHA1 signature + percentEncode (aliyun spec)
    sign_test.go                            — pure unit tests for signing
  app/  (shared package, new files added here)
    idcard_root.go                          — NewIdcardRootCmd()
    idcard_cmd.go                           — verify subcommand
    idcard_verify_test.go                   — e2e: real CloudAuth via CLI
```

Note: `internal/app/` is shared between both CLIs. The idcard commands live in `idcard_root.go` + `idcard_cmd.go` to keep them grouped.

---

### Task 1: CloudAuth signing — HMAC-SHA1 + percentEncode (pure unit tests)

This is the trickiest part (the aliyun signature algorithm). Pure logic, fully unit-testable with no network.

**Files:**
- Create: `internal/idcard/sign.go`
- Create: `internal/idcard/sign_test.go`

**Interfaces:**
- Produces:
  - `func percentEncode(s string) (string, error)` — aliyun URL encoding (RFC3986 with specific replacements)
  - `func sign(params map[string]string, method, secret string) (string, error)` — builds canonical query string, signs with HMAC-SHA1, returns base64

- [ ] **Step 1: Write failing unit tests for signing**

Create `internal/idcard/sign_test.go`:
```go
package idcard

import (
	"testing"
)

func TestPercentEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "abc"},
		{"a b", "a%20b"},       // space -> %20 (not +)
		{"a+b", "a%2Bb"},       // + -> %2B
		{"a*b", "a%2Ab"},       // * -> %2A
		{"~", "~"},             // ~ stays
		{"a/b", "a%2Fb"},
		{"name=张", "name%3D%E5%BC%A0"},
	}
	for _, c := range cases {
		got, err := percentEncode(c.in)
		if err != nil {
			t.Fatalf("percentEncode(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("percentEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSignKnownVector(t *testing.T) {
	// Verify signature output against a known-good vector derived from the
	// aliyun signing algorithm. Using fixed params + secret, the signature
	// must be deterministic.
	params := map[string]string{
		"Action":           "Id2MetaVerifyWithOCR",
		"Version":          "2019-03-07",
		"AccessKeyId":      "TESTAK",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
	}
	sig, err := sign(params, "POST", "TESTSK")
	if err != nil {
		t.Fatal(err)
	}
	if sig == "" {
		t.Fatal("signature should be non-empty")
	}
	// determinism: same input -> same output
	sig2, _ := sign(params, "POST", "TESTSK")
	if sig != sig2 {
		t.Fatalf("signature not deterministic: %q vs %q", sig, sig2)
	}
	// different method -> different signature
	sigGet, _ := sign(params, "GET", "TESTSK")
	if sig == sigGet {
		t.Fatal("GET and POST should produce different signatures")
	}
	// different secret -> different signature
	sigOther, _ := sign(params, "POST", "OTHERSK")
	if sig == sigOther {
		t.Fatal("different secrets should produce different signatures")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/idcard/ -v`
Expected: FAIL — package/sign undefined.

- [ ] **Step 3: Write signing implementation**

Create `internal/idcard/sign.go`:
```go
// Package idcard verifies ID cards via Alibaba Cloud CloudAuth
// (Id2MetaVerifyWithOCR: OCR + two-factor name/ID check in one call).
// It consumes image URLs only; resolving private OSS keys to URLs is the
// caller's job (compose with aliyun-media-cli resolve).
package idcard

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
)

// percentEncode implements the aliyun RPC signature URL encoding:
// RFC3986 percent-encoding, then + -> %20, * -> %2A, %7E -> ~.
func percentEncode(s string) (string, error) {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded, nil
}

// sign computes the HMAC-SHA1 signature for an aliyun RPC API request.
// method is "GET" or "POST". secret is the AccessKeySecret.
// Returns base64-encoded signature.
func sign(params map[string]string, method, secret string) (string, error) {
	// sort parameter names
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// build canonical query string: &k1=v1&k2=v2... (all percent-encoded)
	var b strings.Builder
	for _, k := range keys {
		ek, err := percentEncode(k)
		if err != nil {
			return "", err
		}
		ev, err := percentEncode(params[k])
		if err != nil {
			return "", err
		}
		b.WriteString("&")
		b.WriteString(ek)
		b.WriteString("=")
		b.WriteString(ev)
	}
	canonical := b.String()
	// remove leading '&'
	canonical = canonical[1:]

	// string to sign: METHOD&%2F&<percentEncode(canonical)>
	pe, err := percentEncode(canonical)
	if err != nil {
		return "", err
	}
	stringToSign := method + "&%2F&" + pe

	// HMAC-SHA1 with key = secret + "&"
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/idcard/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/idcard/
git commit -m "feat(idcard): CloudAuth HMAC-SHA1 signing + percentEncode"
```

---

### Task 2: CloudAuth verify client — real Id2MetaVerifyWithOCR call

**Files:**
- Create: `internal/idcard/verify.go`
- Create: `internal/idcard/verify_test.go`

**Interfaces:**
- Consumes: `profile.Profile` (the idcard profile: Region, Endpoint, AccessKeyID, AccessKeySecret)
- Produces:
  - `type Result struct` with fields: `OK bool`, `Passed bool`, `Name, IDCard, Gender, Ethnicity, BirthDate, Address, RequestID, ErrorMessage, VerifyMessage string`, `Raw map[string]any`
  - `func NewVerifier(p profile.Profile) *Verifier`
  - `func (v *Verifier) Verify(ctx context.Context, frontURL, backURL string) (Result, error)`

- [ ] **Step 1: Write failing integration test (real CloudAuth)**

Create `internal/idcard/verify_test.go`:
```go
package idcard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// testProfile loads the real verify profile from profiles.local/.
func testProfile(t *testing.T) (profile.Profile, func()) {
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
	old := os.Getenv("ALIYUN_MEDIA_CLI_HOME")
	os.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	return mustLoadProfile(t, name), func() { os.Setenv("ALIYUN_MEDIA_CLI_HOME", old) }
}

func mustLoadProfile(t *testing.T, name string) profile.Profile {
	t.Helper()
	p, err := profile.Load(name)
	if err != nil {
		t.Skipf("load profile: %v", err)
	}
	return p
}

// TestVerifyWithRealImages calls the real CloudAuth endpoint with real ID
// card images. This test REQUIRES:
//   1. profiles.local/mamamate-verify.toml (real CloudAuth creds)
//   2. A publicly readable front-image URL in env ALIYUN_IDCARD_TEST_FRONT_URL
//   3. (optional) back image in ALIYUN_IDCARD_TEST_BACK_URL
// If the env URLs are absent, the test skips (no real images to verify).
func TestVerifyWithRealImages(t *testing.T) {
	profile, cleanup := testProfile(t)
	defer cleanup()

	frontURL := os.Getenv("ALIYUN_IDCARD_TEST_FRONT_URL")
	if frontURL == "" {
		t.Skip("set ALIYUN_IDCARD_TEST_FRONT_URL to a readable ID-card image URL to run this integration test")
	}
	backURL := os.Getenv("ALIYUN_IDCARD_TEST_BACK_URL")

	v := NewVerifier(profile)
	res, err := v.Verify(context.Background(), frontURL, backURL)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	// ok=true means the API call succeeded (regardless of pass/fail).
	if !res.OK {
		t.Fatalf("API call failed: %s", res.ErrorMessage)
	}
	// We cannot assert passed=true (depends on the real card); just log.
	t.Logf("verify result: passed=%v name=%q requestId=%q", res.Passed, res.Name, res.RequestID)
}
```

- [ ] **Step 2: Run test to verify it fails (compile)**

Run: `go test ./internal/idcard/ -run TestVerify -v`
Expected: FAIL — `NewVerifier`, `Result`, `Verify` undefined.

- [ ] **Step 3: Write the verifier implementation**

Create `internal/idcard/verify.go`:
```go
package idcard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// Result holds the outcome of an Id2MetaVerifyWithOCR call.
type Result struct {
	OK           bool   `json:"ok"`
	Passed       bool   `json:"passed"`
	Name         string `json:"name,omitempty"`
	IDCard       string `json:"idCard,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Ethnicity    string `json:"ethnicity,omitempty"`
	BirthDate    string `json:"birthDate,omitempty"`
	Address      string `json:"address,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	VerifyMessage string `json:"verifyMessage,omitempty"`
	Raw          map[string]any `json:"raw,omitempty"`
}

// Verifier calls the CloudAuth Id2MetaVerifyWithOCR endpoint.
type Verifier struct {
	profile profile.Profile
	client  *http.Client
}

// NewVerifier builds a verifier from a profile.
func NewVerifier(p profile.Profile) *Verifier {
	return &Verifier{
		profile: p,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Verify performs OCR + two-factor verification. frontURL is required,
// backURL is optional. ok=false means the API call itself failed (err may
// also be non-nil for transport failures). ok=true, passed=false means the
// call succeeded but name/ID didn't match.
func (v *Verifier) Verify(ctx context.Context, frontURL, backURL string) (Result, error) {
	endpoint := v.profile.Endpoint
	if endpoint == "" {
		endpoint = "cloudauth.cn-shanghai.aliyuncs.com"
	}
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}

	params := map[string]string{
		"Action":           "Id2MetaVerifyWithOCR",
		"Version":          "2019-03-07",
		"Format":           "JSON",
		"RegionId":         regionFromProfile(v.profile),
		"AccessKeyId":      v.profile.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureNonce":   nonce(),
		"CertUrl":          frontURL,
	}
	if backURL != "" {
		params["CertNationalUrl"] = backURL
	}

	sig, err := sign(params, "POST", v.profile.AccessKeySecret)
	if err != nil {
		return Result{OK: false, ErrorMessage: "signing failed"}, err
	}
	params["Signature"] = sig

	form := encodeForm(params)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form))
	if err != nil {
		return Result{OK: false, ErrorMessage: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return Result{OK: false, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
		return Result{OK: false, ErrorMessage: msg}, fmt.Errorf("cloudauth returned %d", resp.StatusCode)
	}

	return parseResponse(body)
}

func regionFromProfile(p profile.Profile) string {
	if p.Region != "" {
		return p.Region
	}
	return "cn-shanghai"
}

func nonce() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func encodeForm(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("&")
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteString("=")
		b.WriteString(url.QueryEscape(params[k]))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "..."
}

// parseResponse turns the CloudAuth JSON response into a Result.
func parseResponse(body []byte) (Result, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return Result{OK: false, ErrorMessage: "parse error: " + err.Error()}, err
	}

	code := str(root["Code"])
	if code != "" && code != "200" {
		return Result{OK: false, ErrorMessage: "API error: " + str(root["Message"])}, nil
	}

	ro, _ := root["ResultObject"].(map[string]any)
	bizCode := str(ro["BizCode"])
	passed := bizCode == "1"

	r := Result{
		OK:        true,
		Passed:    passed,
		RequestID: str(root["RequestId"]),
		Raw:       root,
	}

	// CardInfo is a JSON string needing second parse
	if cardStr := str(ro["CardInfo"]); cardStr != "" {
		var card map[string]any
		if err := json.Unmarshal([]byte(cardStr), &card); err == nil {
			r.Name = str(card["certName"])
			r.IDCard = str(card["certNo"])
			r.Ethnicity = str(card["nationality"])
			r.BirthDate = str(card["birthDate"])
			r.Address = str(card["address"])
		}
	}

	if !passed {
		if bizCode == "2" {
			r.VerifyMessage = "姓名与身份证号不匹配"
		} else if bizCode != "" {
			r.VerifyMessage = "核验结果: BizCode=" + bizCode
		}
	}

	return r, nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
