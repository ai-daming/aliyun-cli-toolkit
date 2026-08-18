package oss

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// testProfile loads a real profile for integration testing from profiles.local/.
// Tests skip if the profile fixture is unavailable (no credentials / no network).
func testProfile(t *testing.T) profile.Profile {
	t.Helper()
	name := os.Getenv("ALIYUN_MEDIA_CLI_TEST_PROFILE")
	if name == "" {
		name = "mamamate"
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
	p, err := profile.Load(name)
	if err != nil {
		t.Skipf("cannot load profile: %v", err)
	}
	p.Name = name
	return p
}

func uniqueKey(prefix string) string {
	return fmt.Sprintf("test-cli/%s-%d", prefix, time.Now().UnixNano())
}

func TestUploadPublicRoundTrip(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}

	key := uniqueKey("upload-public")
	t.Cleanup(func() { _ = c.Delete(context.Background(), key) })

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
	key := uniqueKey("upload-private")
	t.Cleanup(func() { _ = c.Delete(context.Background(), key) })

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
	if res.ETag == "" {
		t.Fatal("etag should still be present for private upload")
	}
}

func TestResolvePrivateReturnsSignedURL(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := uniqueKey("resolve-private")
	t.Cleanup(func() { _ = c.Delete(context.Background(), key) })

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
	if !strings.Contains(strings.ToLower(url), "signature=") {
		t.Fatalf("signed url has no signature param: %s", url)
	}
}

func TestStatExistsAndMissing(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := uniqueKey("stat-exists")
	t.Cleanup(func() { _ = c.Delete(context.Background(), key) })

	payload := []byte("stat me\n")
	if _, err := c.Upload(context.Background(), key, payload, "text/plain", false); err != nil {
		t.Fatal(err)
	}

	exists, size, ct, etag, err := c.Stat(context.Background(), key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !exists {
		t.Fatal("should exist")
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}
	// Note: OSS HEAD (GetObjectMeta) does not return Content-Type, so ct may be empty.
	_ = ct
	if etag == "" {
		t.Fatal("etag should be non-empty")
	}

	// missing key
	exists2, _, _, _, err := c.Stat(context.Background(), uniqueKey("stat-missing"))
	if err != nil {
		t.Fatalf("Stat missing should not error: %v", err)
	}
	if exists2 {
		t.Fatal("missing key should report exists=false")
	}
}

func TestDeleteRemovesObject(t *testing.T) {
	p := testProfile(t)
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	key := uniqueKey("delete")
	payload := []byte("delete me\n")
	if _, err := c.Upload(context.Background(), key, payload, "text/plain", false); err != nil {
		t.Fatal(err)
	}
	// confirm exists
	exists, _, _, _, _ := c.Stat(context.Background(), key)
	if !exists {
		t.Fatal("object should exist before delete")
	}
	if err := c.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	existsAfter, _, _, _, _ := c.Stat(context.Background(), key)
	if existsAfter {
		t.Fatal("object should be gone after delete")
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
	if !strings.Contains(url, "some/key.jpg") {
		t.Fatalf("public url should contain the key: %s", url)
	}
	// when profile has a public-domain, url must use it
	if !strings.HasPrefix(url, "https://"+p.PublicDomain) {
		t.Fatalf("public url should start with public domain %q: %s", p.PublicDomain, url)
	}
}

// TestAssumeRoleBadCredentials exercises the AssumeRole error path with fake
// credentials. The real STS endpoint rejects invalid AK/SK, so assumeRole
// returns a non-nil error. This covers the defensive error branches.
func TestAssumeRoleBadCredentials(t *testing.T) {
	// Build a client with a real-shaped but invalid profile. We reuse the real
	// profile's endpoint/region but swap credentials, so STS genuinely rejects.
	base := testProfile(t)
	bad := base
	bad.AccessKeyID = "LTAI000000000000FAKE"
	bad.AccessKeySecret = "0000000000000000000000000000000000000000FAKE"
	c, err := NewClient(bad)
	if err != nil {
		t.Fatal(err)
	}

	// Upload must fail because AssumeRole fails first.
	_, err = c.Upload(context.Background(), uniqueKey("bad-creds"), []byte("x"), "text/plain", false)
	if err == nil {
		t.Fatal("Upload with bad credentials should fail")
	}

	// ResolvePrivate must fail for the same reason.
	_, err = c.ResolvePrivate(context.Background(), uniqueKey("bad-creds"), 5*time.Second)
	if err == nil {
		t.Fatal("ResolvePrivate with bad credentials should fail")
	}

	// Stat must fail too.
	_, _, _, _, err = c.Stat(context.Background(), uniqueKey("bad-creds"))
	if err == nil {
		t.Fatal("Stat with bad credentials should fail")
	}

	// Delete must fail too.
	err = c.Delete(context.Background(), uniqueKey("bad-creds"))
	if err == nil {
		t.Fatal("Delete with bad credentials should fail")
	}
}

// TestUploadToNonexistentBucket covers the OSS put error path: valid creds but
// a bucket the role can't access. We point at a bucket name that almost
// certainly doesn't exist / isn't authorized, so PutObject fails.
func TestUploadToInaccessibleBucket(t *testing.T) {
	p := testProfile(t)
	bad := p
	bad.Bucket = "aliyun-cli-toolkit-nonexistent-bucket-xyz"
	c, err := NewClient(bad)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Upload(context.Background(), uniqueKey("no-bucket"), []byte("x"), "text/plain", false)
	if err == nil {
		t.Fatal("Upload to inaccessible bucket should fail")
	}
}

// TestOperationsFailWithInvalidBucketName proves invalid profile state is
// rejected before STS or OSS receives a request.
func TestOperationsFailWithInvalidBucketName(t *testing.T) {
	p := testProfile(t)
	bad := p
	bad.Bucket = "INVALID-UPPERCASE-Bucket" // OSS rejects uppercase in bucket names
	if _, err := NewClient(bad); err == nil {
		t.Fatal("NewClient should reject invalid bucket name")
	}
}
