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
		key, "image/jpeg", "private", 2097152, now, creds)
	if err != nil {
		t.Fatal(err)
	}
	if form.URL != "https://oss.mamamate.cn" || form.Fields["key"] != key ||
		form.Fields["x-oss-object-acl"] != "private" || form.Fields["x-oss-forbid-overwrite"] != "true" ||
		form.Fields["x-oss-content-type"] != "image/jpeg" || form.Fields["Content-Type"] != "" {
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
		{"eq", "$x-oss-content-type", "image/jpeg"},
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
	if _, err := client.SignBrowserUpload(context.Background(), key, "image/jpeg", 60, true); err != nil {
		t.Fatal(err)
	}
	client.prof.Endpoint = localEndpoint
	details, err := client.InspectBrowserUpload(context.Background(), key)
	if err != nil || !details.Exists || details.Size != 60 || details.ContentType != "image/jpeg" || len(details.Header) != 32 {
		t.Fatalf("unexpected inspection: %+v, %v", details, err)
	}
	policies, calls := recorder.snapshot()
	if len(policies) != 3 || len(calls) != 2 {
		t.Fatalf("unexpected policy or storage calls: %v, %v", policies, calls)
	}
	for i, want := range []struct{ action, acl string }{
		{"oss:PutObject", "default"},
		{"oss:PutObject", "private"},
		{"oss:GetObject", ""},
	} {
		var policy sessionPolicy
		if err := json.Unmarshal([]byte(policies[i]), &policy); err != nil {
			t.Fatal(err)
		}
		if len(policy.Statement) != 1 {
			t.Fatalf("expected one statement: %s", policies[i])
		}
		stmt := policy.Statement[0]
		resource := "acs:oss:*:*:" + client.prof.Bucket + "/" + key
		if !reflect.DeepEqual(stmt.Action, []string{want.action}) || !reflect.DeepEqual(stmt.Resource, []string{resource}) {
			t.Fatalf("unexpected scope: %s", policies[i])
		}
		if want.acl != "" {
			if stmt.Condition == nil || stmt.Condition.StringEquals["oss:x-oss-object-acl"] != want.acl {
				t.Fatalf("expected exact ACL %q: %s", want.acl, policies[i])
			}
		} else if stmt.Condition != nil {
			t.Fatalf("read should have no upload conditions: %s", policies[i])
		}
	}
}

func TestBrowserUploadRejectsInvalidInputBeforeAssumingRole(t *testing.T) {
	client, recorder := newProtocolTestClient(t, http.NotFoundHandler())
	const key = "provider-media/tenant/profile/gallery/photo.jpg"
	ctx := context.Background()
	for _, args := range []struct {
		key, contentType string
		size             int64
	}{
		{"../photo.jpg", "image/jpeg", 1},
		{key, "image/jpeg", 0},
		{key, "image/jpeg", maxBrowserUploadBytes + 1},
		{key, "image/svg+xml", 1},
	} {
		if _, err := client.SignBrowserUpload(ctx, args.key, args.contentType, args.size, false); err == nil {
			t.Fatalf("accepted invalid upload: %+v", args)
		}
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := client.SignBrowserUpload(canceled, key, "image/jpeg", 1, false); err != context.Canceled {
		t.Fatalf("sign with canceled context: %v", err)
	}
	if _, err := client.InspectBrowserUpload(canceled, key); err != context.Canceled {
		t.Fatalf("inspect with canceled context: %v", err)
	}
	if policies, calls := recorder.snapshot(); len(policies)+len(calls) != 0 {
		t.Fatalf("invalid or canceled request contacted STS/OSS: %v / %v", policies, calls)
	}
}

func TestInspectBrowserUploadHandlesMissingOrReplacedObjects(t *testing.T) {
	for _, tt := range []struct {
		name       string
		headStatus int
		withETag   bool
		wantCalls  int
		wantError  bool
	}{
		{"missing", http.StatusNotFound, false, 1, false},
		{"denied", http.StatusForbidden, false, 1, true},
		{"missing ETag", http.StatusOK, false, 1, true},
		{"replaced after HEAD", http.StatusOK, true, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					if tt.headStatus == http.StatusOK {
						w.Header().Set("Content-Length", "3")
						if tt.withETag {
							w.Header().Set("ETag", "\"initial\"")
						}
					}
					w.WriteHeader(tt.headStatus)
					return
				}
				if r.Header.Get("If-Match") != "\"initial\"" {
					t.Errorf("missing original object condition: %q", r.Header.Get("If-Match"))
				}
				w.WriteHeader(http.StatusPreconditionFailed)
			}))
			result, err := client.InspectBrowserUpload(context.Background(), "provider-media/tenant/profile/gallery/photo.jpg")
			if result.Exists || (err != nil) != tt.wantError {
				t.Fatalf("unexpected inspection: %+v, %v", result, err)
			}
			if _, calls := recorder.snapshot(); len(calls) != tt.wantCalls {
				t.Fatalf("unexpected OSS calls: %v", calls)
			}
		})
	}
}
