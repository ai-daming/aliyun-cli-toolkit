package oss

import (
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// These tests exercise pure-logic branches of the client that don't need
// network or credentials, pushing coverage of defensive paths past 85%.

func TestNewClientRequiresRegion(t *testing.T) {
	// When Region is empty, NewClient must fail fast — a silent region default
	// would route OSS calls to the wrong endpoint. (No network call here.)
	_, err := NewClient(profile.Profile{
		Name:            "noregion",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	})
	if err == nil {
		t.Fatal("NewClient with empty region should return an error")
	}
}

func TestNewClientRejectsIncompleteOrUnsafeProfile(t *testing.T) {
	base := profile.Profile{
		Name:            "component",
		Bucket:          "component-bucket",
		Region:          "cn-test",
		RoleArn:         "acs:ram::123:role/test",
		AccessKeyID:     "TEST_ACCESS_KEY",
		AccessKeySecret: "TEST_ACCESS_SECRET",
	}
	tests := []struct {
		name   string
		mutate func(*profile.Profile)
	}{
		{name: "unsafe bucket", mutate: func(p *profile.Profile) { p.Bucket = "bucket*" }},
		{name: "missing role ARN", mutate: func(p *profile.Profile) { p.RoleArn = "" }},
		{name: "missing access key ID", mutate: func(p *profile.Profile) { p.AccessKeyID = "" }},
		{name: "missing access key secret", mutate: func(p *profile.Profile) { p.AccessKeySecret = "" }},
		{name: "plaintext endpoint", mutate: func(p *profile.Profile) { p.Endpoint = "http://oss.example.test" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			tt.mutate(&p)
			if _, err := NewClient(p); err == nil {
				t.Fatal("NewClient should reject invalid profile")
			}
		})
	}
}

func TestOssEndpointDefaultsWhenEmpty(t *testing.T) {
	c := &Client{prof: profile.Profile{Region: "cn-hangzhou"}}
	if got := c.ossEndpoint(); got != "oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("ossEndpoint = %q, want default", got)
	}
	c2 := &Client{prof: profile.Profile{Region: "cn-hangzhou", Endpoint: "custom.example.com"}}
	if got := c2.ossEndpoint(); got != "custom.example.com" {
		t.Fatalf("ossEndpoint = %q, want custom", got)
	}
}

func TestPublicURLWithoutDomain(t *testing.T) {
	c := &Client{prof: profile.Profile{Bucket: "bkt", Region: "cn-huhehaote"}}
	url := c.PublicURL("obj/key.jpg")
	want := "https://bkt.oss-cn-huhehaote.aliyuncs.com/obj/key.jpg"
	if url != want {
		t.Fatalf("PublicURL no-domain = %q, want %q", url, want)
	}
}

func TestPublicURLWithDomain(t *testing.T) {
	c := &Client{prof: profile.Profile{Bucket: "bkt", PublicDomain: "cdn.x.com"}}
	url := c.PublicURL("obj/key.jpg")
	want := "https://cdn.x.com/obj/key.jpg"
	if url != want {
		t.Fatalf("PublicURL with-domain = %q, want %q", url, want)
	}
}

func TestIsNotFoundVariants(t *testing.T) {
	// non-ServiceError with NoSuchKey in message
	if !isNotFound(strErr("Get fail: NoSuchKey not found")) {
		t.Fatal("NoSuchKey message should be detected as not-found")
	}
	// non-ServiceError with 404 in message
	if !isNotFound(strErr("GET 404")) {
		t.Fatal("404 message should be detected as not-found")
	}
	// clearly not found-ish
	if isNotFound(strErr("some other error")) {
		t.Fatal("unrelated error should not be not-found")
	}

	// ServiceError 404 status
	if !isNotFound(oss.ServiceError{StatusCode: 404, Code: "NoSuchKey"}) {
		t.Fatal("404 ServiceError should be not-found")
	}
	// ServiceError with NoSuch in Code but non-404 status
	if !isNotFound(oss.ServiceError{StatusCode: 400, Code: "NoSuchObject"}) {
		t.Fatal("ServiceError with NoSuch code should be not-found")
	}
	// ServiceError that is NOT not-found
	if isNotFound(oss.ServiceError{StatusCode: 403, Code: "AccessDenied"}) {
		t.Fatal("AccessDenied ServiceError should not be not-found")
	}
}

type strErr string

func (e strErr) Error() string { return string(e) }
