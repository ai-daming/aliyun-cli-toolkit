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
	Name            string `toml:"-" json:"name"`
	Bucket          string `toml:"bucket" json:"bucket"`
	Region          string `toml:"region" json:"region"`
	Endpoint        string `toml:"endpoint" json:"endpoint"`
	RoleArn         string `toml:"role-arn" json:"role-arn"`
	AccessKeyID     string `toml:"access-key-id" json:"access-key-id"`
	AccessKeySecret string `toml:"access-key-secret" json:"access-key-secret"`
	PublicDomain    string `toml:"public-domain,omitempty" json:"public-domain,omitempty"`
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
