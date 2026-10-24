package app

import (
	"bytes"
	"encoding/json"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
	"net/url"
	"testing"
)

func TestDedicatedReadProfileCommandRoundTrip(t *testing.T) {
	t.Setenv("ALIYUN_MEDIA_CLI_HOME", t.TempDir())
	add := NewRootCmd()
	add.SetOut(&bytes.Buffer{})
	add.SetArgs([]string{"profile", "add", "meal-read", "--bucket", "mamamate", "--region", "cn-huhehaote", "--access-key-id", "FAKE_AK", "--access-key-secret", "FAKE_SECRET", "--signing-mode", "direct-read", "--read-prefix", "xiaohe-meal-show/"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}
	p, err := profile.Load("meal-read")
	if err != nil || p.SigningMode != "direct-read" {
		t.Fatal("profile not persisted")
	}
	root := NewRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"resolve", "--profile", "meal-read", "--key", "xiaohe-meal-show/a.png", "--ttl", "168h"})
	if err = root.Execute(); err != nil {
		t.Fatal(err)
	}
	var result struct {
		URL        string `json:"url"`
		Visibility string `json:"visibility"`
		ExpiresAt  string `json:"expiresAt"`
	}
	if err = json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(result.URL)
	if result.Visibility != "private" || result.ExpiresAt == "" || u.Query().Get("x-oss-expires") != "604800" {
		t.Fatal("invalid result")
	}
	for _, args := range [][]string{
		{"resolve", "--profile", "meal-read", "--key", "private/secret.png", "--ttl", "168h"},
		{"delete", "--profile", "meal-read", "--key", "xiaohe-meal-show/a.png"},
		{"stat", "--profile", "meal-read", "--key", "xiaohe-meal-show/a.png"},
		{"list", "--profile", "meal-read", "--prefix", "xiaohe-meal-show/"},
	} {
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatal("read-only boundary bypassed")
		}
	}
}
