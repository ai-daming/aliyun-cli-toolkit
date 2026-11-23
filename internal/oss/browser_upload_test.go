package oss

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUploadFormBindsOneObjectAndExactFileSize(t *testing.T) {
	now := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	const key = "provider-private-media/tenant/profile/identity/front/image.jpg"
	creds := stsCreds{ak: "temporary-id", sk: "temporary-secret", token: "test-token"}
	form, err := buildUploadForm("mamamate", "cn-huhehaote", "https://oss.mamamate.cn",
		key, "image/jpeg", 2097152, true, now, creds)
	if err != nil {
		t.Fatal(err)
	}
	if form.URL != "https://oss.mamamate.cn" || form.Fields["key"] != key ||
		form.Fields["x-oss-object-acl"] != "private" || form.Fields["x-oss-forbid-overwrite"] != "true" {
		t.Fatalf("unexpected upload fields: %+v", form)
	}
	raw, err := base64.StdEncoding.DecodeString(form.Fields["policy"])
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Expiration time.Time
		Conditions []any
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		t.Fatal(err)
	}
	if !policy.Expiration.Equal(now.Add(10*time.Minute)) || !form.ExpiresAt.Equal(policy.Expiration) {
		t.Fatalf("unexpected expiry %v %v", policy.Expiration, form.ExpiresAt)
	}
	for _, want := range [][]any{
		{"eq", "$key", key},
		{"content-length-range", float64(2097152), float64(2097152)},
		{"eq", "$content-type", "image/jpeg"},
		{"eq", "$x-oss-object-acl", "private"},
		{"eq", "$x-oss-forbid-overwrite", "true"},
	} {
		found := false
		for _, condition := range policy.Conditions {
			found = found || reflect.DeepEqual(condition, want)
		}
		if !found {
			t.Errorf("missing policy condition %v", want)
		}
	}
	if len(form.Fields["x-oss-signature"]) != 64 || strings.Contains(string(raw), creds.sk) {
		t.Fatal("invalid signature or leaked signing key")
	}
}

func TestBrowserUploadUsesSeparateObjectScopedWriteAndRead(t *testing.T) {
	const key = "provider-media/tenant/profile/gallery/image.jpg"
	client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"example"`)
		if request.Method == http.MethodHead {
			w.Header().Set("Content-Length", "60")
			return
		}
		if request.Method != http.MethodGet || request.Header.Get("Range") != "bytes=0-31" ||
			request.Header.Get("If-Match") != `"example"` {
			t.Errorf("unexpected bounded read: %s %s", request.Method, request.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 0-31/60")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = fmt.Fprint(w, strings.Repeat("x", 32))
	}))
	localEndpoint := client.prof.Endpoint
	client.prof.Endpoint = "oss-cn-hangzhou-internal.aliyuncs.com"
	form, err := client.SignBrowserUpload(context.Background(), key, "image/jpeg", 60, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://" + client.prof.Bucket + ".oss-" + client.prof.Region + ".aliyuncs.com"; form.URL != want {
		t.Fatalf("browser upload must use the region's public endpoint: got %q, want %q", form.URL, want)
	}
	client.prof.Endpoint = localEndpoint
	details, err := client.InspectBrowserUpload(context.Background(), key)
	if err != nil || !details.Exists || details.Size != 60 || details.ContentType != "image/jpeg" || len(details.Header) != 32 {
		t.Fatalf("unexpected inspection: %+v, %v", details, err)
	}
	policies, calls := recorder.snapshot()
	if len(policies) != 2 || !strings.Contains(policies[0], "oss:PutObject") ||
		!strings.Contains(policies[1], "oss:GetObject") || !strings.Contains(policies[0], key) ||
		!strings.Contains(policies[1], key) || len(calls) != 2 {
		t.Fatalf("unexpected scope or storage calls: %v, %v", policies, calls)
	}
}
