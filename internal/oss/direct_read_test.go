package oss

import (
	"context"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
	"net/url"
	"testing"
	"time"
)

func readProfile() profile.Profile {
	return profile.Profile{Name: "meal-read", Bucket: "mamamate", Region: "cn-huhehaote", AccessKeyID: "TEST_AK", AccessKeySecret: "TEST_SECRET", SigningMode: "direct-read", ReadPrefix: "xiaohe-meal-show/"}
}
func TestDirectReadSignsSevenDaysWithoutSTS(t *testing.T) {
	c, err := NewClient(readProfile())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := c.ResolvePrivate(context.Background(), "xiaohe-meal-show/tenant/provider/image.png", 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme != "https" || q.Get("x-oss-expires") != "604800" || q.Get("x-oss-signature-version") != "OSS4-HMAC-SHA256" || q.Get("x-oss-signature") == "" {
		t.Fatal("incorrect signed GET")
	}
	if q.Has("security-token") || q.Has("x-oss-security-token") {
		t.Fatal("temporary credentials used")
	}
}
func TestDirectReadRejectsOtherKeysAndInvalidLifetime(t *testing.T) {
	c, err := NewClient(readProfile())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"provider-media/private.jpg", "xiaohe-meal-show-other/a.png", "xiaohe-meal-show/../secret", "xiaohe-meal-show/"} {
		if _, err = c.ResolvePrivate(context.Background(), key, time.Hour); err == nil {
			t.Fatalf("accepted %s", key)
		}
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Millisecond, 7*24*time.Hour + time.Second} {
		if _, err = c.ResolvePrivate(context.Background(), "xiaohe-meal-show/a.png", ttl); err == nil {
			t.Fatal("accepted invalid lifetime")
		}
	}
	if _, err = c.assumeRole("{}"); err == nil {
		t.Fatal("read-only profile allowed STS operation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.ResolvePrivate(ctx, "xiaohe-meal-show/a.png", time.Hour); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestDirectReadProfileFailsClosed(t *testing.T) {
	for _, change := range []func(*profile.Profile){
		func(p *profile.Profile) { p.ReadPrefix = "" }, func(p *profile.Profile) { p.ReadPrefix = "xiaohe-meal-show" },
		func(p *profile.Profile) { p.ReadPrefix = "../" }, func(p *profile.Profile) { p.RoleArn = "role" },
		func(p *profile.Profile) { p.SigningMode = "unknown" },
	} {
		p := readProfile()
		change(&p)
		if _, err := NewClient(p); err == nil {
			t.Fatal("accepted invalid signing profile")
		}
	}
	p := readProfile()
	p.SigningMode = ""
	p.ReadPrefix = ""
	p.RoleArn = "role"
	c, err := NewClient(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ResolvePrivate(context.Background(), "xiaohe-meal-show/a.png", 7*24*time.Hour); err == nil {
		t.Fatal("STS accepted seven-day TTL")
	}
}
