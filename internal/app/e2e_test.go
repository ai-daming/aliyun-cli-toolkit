package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// setupIntegrationHome copies profiles.local/<profile>.toml into a temp dir's
// profiles/ subdir so profile.Load works. Skips if the fixture is absent.
func setupIntegrationHome(t *testing.T) string {
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
	return name
}

func uniqueKey(prefix string) string {
	return fmt.Sprintf("test-cli/%s-%d", prefix, time.Now().UnixNano())
}

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
