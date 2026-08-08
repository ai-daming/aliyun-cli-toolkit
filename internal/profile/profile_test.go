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

func TestLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	badPath := filepath.Join(dir, "profiles", "bad.toml")
	if err := os.MkdirAll(filepath.Dir(badPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("this is not = = valid toml {{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load("bad")
	if err == nil {
		t.Fatal("expected parse error for corrupt profile")
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

func TestListEmptyWhenNoProfilesDir(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	names, err := List()
	if err != nil {
		t.Fatalf("List on missing dir: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty list, got %v", names)
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

func TestDeleteMissing(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	err := Delete("never-existed")
	if err == nil {
		t.Fatal("expected error deleting missing profile")
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

func TestConfigDirDefaultFallback(t *testing.T) {
	// When ALIYUN_MEDIA_CLI_HOME is unset, fall back to ~/.config/aliyun-media-cli.
	// We can't safely unset HOME on all systems, but we can verify the env override
	// path is taken when set, and the default path contains the expected suffix.
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "aliyun-media-cli")
	if dir != want {
		t.Fatalf("ConfigDir = %q, want %q", dir, want)
	}
}

func TestSaveFailsWhenMkdirBlocked(t *testing.T) {
	// Point home at a path whose parent we make unwritable, so MkdirAll fails.
	parent := t.TempDir()
	target := filepath.Join(parent, "locked")
	if err := os.MkdirAll(target, 0o500); err != nil { // read+execute, no write
		t.Fatal(err)
	}
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", target)
	// restore perms so cleanup works
	t.Cleanup(func() { _ = os.Chmod(target, 0o700) })

	err := (Profile{Name: "x", Bucket: "b"}).Save()
	if err == nil {
		t.Fatal("expected Save to fail when profiles dir cannot be created")
	}
}

func TestIgnoresNonTomlFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", dir)
	profilesDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(profilesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// a stray non-toml file should be ignored
	if err := os.WriteFile(filepath.Join(profilesDir, "README.md"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Profile{Name: "real"}.Save()); err != nil {
		t.Fatal(err)
	}
	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "real" {
		t.Fatalf("List = %v, want [real]", names)
	}
}
