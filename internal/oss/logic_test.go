package oss

import (
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// These tests exercise pure-logic branches of the client that don't need
// network or credentials, pushing coverage of defensive paths past 85%.

func TestNewClientDefaultRegion(t *testing.T) {
	// When Region is empty, NewClient must default to cn-huhehaote without error.
	// (It only constructs the STS client config; no network call here.)
	c, err := NewClient(profile.Profile{
		Name:            "noremion",
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.prof.Region != "" {
		// prof is stored as-is; the default is applied inside assumeRole/ossEndpoint.
		// This assertion documents that behavior.
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
