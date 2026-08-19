package oss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	sts "github.com/alibabacloud-go/sts-20150401/client"
	sdkoss "github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

type protocolRecorder struct {
	mu          sync.Mutex
	policies    []string
	ossRequests []string
}

func (r *protocolRecorder) addPolicy(policy string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies = append(r.policies, policy)
}

func (r *protocolRecorder) addOSSRequest(request string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ossRequests = append(r.ossRequests, request)
}

func (r *protocolRecorder) snapshot() ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.policies...), append([]string(nil), r.ossRequests...)
}

func newProtocolTestClient(t *testing.T, ossHandler http.Handler) (*Client, *protocolRecorder) {
	return newProtocolTestClientWithSTSBody(t, `{"RequestId":"request-id","AssumedRoleUser":{"Arn":"acs:ram::123:role/test","AssumedRoleId":"test"},"Credentials":{"SecurityToken":"TEST_SECURITY_TOKEN","AccessKeyId":"TEST_TEMP_ACCESS_KEY","AccessKeySecret":"TEST_TEMP_ACCESS_SECRET","Expiration":"2099-01-01T00:00:00Z"}}`, ossHandler)
}

func newProtocolTestClientWithSTSBody(t *testing.T, stsBody string, ossHandler http.Handler) (*Client, *protocolRecorder) {
	t.Helper()
	recorder := &protocolRecorder{}
	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		recorder.addPolicy(request.Form.Get("Policy"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, stsBody)
	}))
	t.Cleanup(stsServer.Close)

	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		recorder.addOSSRequest(request.Method + " " + request.URL.RequestURI())
		ossHandler.ServeHTTP(w, request)
	}))
	t.Cleanup(ossServer.Close)

	stsURL, err := url.Parse(stsServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	protocol := "HTTP"
	region := "cn-test"
	ak, sk := "TEST_ACCESS_KEY", "TEST_ACCESS_SECRET"
	endpoint := stsURL.Host
	stsClient, err := sts.NewClient(&openapi.Config{
		AccessKeyId:     &ak,
		AccessKeySecret: &sk,
		RegionId:        &region,
		Endpoint:        &endpoint,
		Protocol:        &protocol,
	})
	if err != nil {
		t.Fatal(err)
	}

	return &Client{
		prof: profile.Profile{
			Name:            "component-test",
			Bucket:          "component-bucket",
			Region:          region,
			Endpoint:        ossServer.URL,
			RoleArn:         "acs:ram::123:role/test",
			AccessKeyID:     ak,
			AccessKeySecret: sk,
		},
		stsClient: stsClient,
		ossClientOptions: []sdkoss.ClientOption{
			sdkoss.ForcePathStyle(true),
			sdkoss.HTTPClient(ossServer.Client()),
		},
	}, recorder
}

func TestListUsesRealSTSAndOSSSDKProtocol(t *testing.T) {
	client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s", request.Method)
		}
		query := request.URL.Query()
		if query.Get("list-type") != "2" || query.Get("prefix") != "媒体/" || query.Get("max-keys") != "2" || query.Get("continuation-token") != "cursor-1" {
			t.Errorf("query = %#v", query)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Prefix>%E5%AA%92%E4%BD%93%2F</Prefix>
  <ContinuationToken>cursor-1</ContinuationToken>
  <MaxKeys>2</MaxKeys>
  <IsTruncated>true</IsTruncated>
  <NextContinuationToken>cursor-2</NextContinuationToken>
  <EncodingType>url</EncodingType>
  <Contents>
    <Key>%E5%AA%92%E4%BD%93%2Fexample.jpg</Key>
    <LastModified>2026-08-18T18:00:00+08:00</LastModified>
    <ETag>&quot;etag&quot;</ETag>
    <Size>123</Size>
  </Contents>
</ListBucketResult>`)
	}))

	result, err := client.List(context.Background(), "媒体/", "cursor-1", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Objects) != 1 {
		t.Fatalf("objects = %#v", result.Objects)
	}
	object := result.Objects[0]
	if object.Key != "媒体/example.jpg" || object.LastModified != "2026-08-18T10:00:00Z" || object.Size != 123 || object.ETag != "etag" {
		t.Fatalf("object = %#v", object)
	}
	if result.NextCursor == nil || *result.NextCursor != "cursor-2" {
		t.Fatalf("nextCursor = %#v", result.NextCursor)
	}

	policies, requests := recorder.snapshot()
	if len(policies) != 1 || len(requests) != 1 {
		t.Fatalf("policies=%d requests=%#v", len(policies), requests)
	}
	statement := onlyStatement(t, decodePolicy(t, policies[0]))
	wantCondition := map[string]any{"StringEquals": map[string]any{"oss:Prefix": "媒体/"}}
	if got, _ := statement["Condition"].(map[string]any); fmt.Sprint(got) != fmt.Sprint(wantCondition) {
		t.Fatalf("Condition = %#v", statement["Condition"])
	}
}

func TestListAcceptsURLDecodedEchoOfOpaqueCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
		echo   string
	}{
		{name: "plus", cursor: "opaque+token", echo: "opaque%2Btoken"},
		{name: "percent", cursor: "opaque%token", echo: "opaque%25token"},
		{name: "unicode", cursor: "游标-token", echo: "%E6%B8%B8%E6%A0%87-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if got := request.URL.Query().Get("continuation-token"); got != tt.cursor {
					t.Errorf("continuation-token = %q", got)
				}
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprintf(w, `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>%s</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`, tt.echo)
			}))

			if _, err := client.List(context.Background(), "media/", tt.cursor, 1); err != nil {
				t.Fatalf("List: %v", err)
			}
		})
	}
}

func TestDeleteUsesRealSTSAndOSSSDKProtocolIdempotently(t *testing.T) {
	client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			t.Errorf("method = %s", request.Method)
		}
		if request.URL.Path != "/component-bucket/media/example.jpg" {
			t.Errorf("path = %q", request.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	for range 2 {
		if err := client.Delete(context.Background(), "media/example.jpg"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	}

	policies, requests := recorder.snapshot()
	if len(policies) != 2 || len(requests) != 2 {
		t.Fatalf("policies=%d requests=%#v", len(policies), requests)
	}
	for _, encoded := range policies {
		statement := onlyStatement(t, decodePolicy(t, encoded))
		body, _ := json.Marshal(statement)
		if !strings.Contains(string(body), `"oss:DeleteObject"`) || !strings.Contains(string(body), `"acs:oss:*:*:component-bucket/media/example.jpg"`) {
			t.Fatalf("delete policy = %s", body)
		}
	}
}

func TestListRejectsInvalidProviderResponseFailClosed(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "outside prefix",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated><Contents><Key>other/file</Key><LastModified>2026-08-18T10:00:00Z</LastModified><ETag>etag</ETag><Size>1</Size></Contents></ListBucketResult>`,
		},
		{
			name: "negative size",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated><Contents><Key>media/file</Key><LastModified>2026-08-18T10:00:00Z</LastModified><ETag>etag</ETag><Size>-1</Size></Contents></ListBucketResult>`,
		},
		{
			name: "missing next cursor",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>true</IsTruncated></ListBucketResult>`,
		},
		{
			name: "repeated cursor",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>true</IsTruncated><NextContinuationToken>cursor-1</NextContinuationToken></ListBucketResult>`,
		},
		{
			name: "missing echoed prefix",
			xml:  `<ListBucketResult><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`,
		},
		{
			name: "overlong next cursor",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>true</IsTruncated><NextContinuationToken>` + strings.Repeat("x", 4097) + `</NextContinuationToken></ListBucketResult>`,
		},
		{
			name: "too many objects",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>cursor-1</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated><Contents><Key>media/one</Key><LastModified>2026-08-18T10:00:00Z</LastModified><ETag>one</ETag><Size>1</Size></Contents><Contents><Key>media/two</Key><LastModified>2026-08-18T10:00:00Z</LastModified><ETag>two</ETag><Size>1</Size></Contents></ListBucketResult>`,
		},
		{
			name: "missing echoed cursor",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`,
		},
		{
			name: "wrong echoed cursor",
			xml:  `<ListBucketResult><Prefix>media/</Prefix><ContinuationToken>other-cursor</ContinuationToken><MaxKeys>1</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`,
		},
		{
			name: "malformed XML",
			xml:  `<ListBucketResult><Contents>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = fmt.Fprint(w, tt.xml)
			}))
			_, err := client.List(context.Background(), "media/", "cursor-1", 1)
			if err == nil || err.Error() != "INVALID_OSS_RESPONSE" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestListProtocolReturnsEmptyPageAndNullCursor(t *testing.T) {
	client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Has("continuation-token") {
			t.Error("first page must not send a continuation token")
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<ListBucketResult><Prefix>media</Prefix><MaxKeys>200</MaxKeys><IsTruncated>false</IsTruncated></ListBucketResult>`)
	}))
	result, err := client.List(context.Background(), "media", "", 200)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Objects == nil || len(result.Objects) != 0 {
		t.Fatalf("objects = %#v", result.Objects)
	}
	if result.NextCursor != nil {
		t.Fatalf("nextCursor = %#v", result.NextCursor)
	}
}

func TestListClassifiesOSSServiceFailureWithoutLeakingResponse(t *testing.T) {
	secretCanary := "PROVIDER_RESPONSE_SECRET_CANARY"
	client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, `<Error><Code>AccessDenied</Code><Message>%s</Message><RequestId>request-id</RequestId></Error>`, secretCanary)
	}))
	_, err := client.List(context.Background(), "media/", "", 1)
	if err == nil || err.Error() != ErrorList {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", err), secretCanary) ||
		strings.Contains(fmt.Sprintf("%#v", err), secretCanary) || errors.Unwrap(err) != nil {
		t.Fatalf("error leaks provider response: %+v", err)
	}
}

func TestListRejectsIncompleteSTSCredentialsWithoutPanicOrLeak(t *testing.T) {
	secretCanary := "STS_RESPONSE_SECRET_CANARY"
	stsBody := `{"RequestId":"request-id","Credentials":{"SecurityToken":"` + secretCanary + `","AccessKeyId":"TEST_TEMP_ACCESS_KEY"}}`
	client, _ := newProtocolTestClientWithSTSBody(t, stsBody, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("OSS must not be called when STS credentials are incomplete")
	}))
	_, err := client.List(context.Background(), "media/", "", 1)
	if err == nil || err.Error() != ErrorSTS {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", err), secretCanary) ||
		strings.Contains(fmt.Sprintf("%#v", err), secretCanary) || errors.Unwrap(err) != nil {
		t.Fatalf("error leaks STS response: %+v", err)
	}
}

func TestListDeleteRejectInvalidLibraryInputsBeforeNetwork(t *testing.T) {
	client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("OSS must not be called for invalid list inputs")
	}))
	for _, tt := range []struct {
		name   string
		prefix string
		cursor string
		limit  int
	}{
		{name: "unsafe prefix", prefix: "media/*", limit: 1},
		{name: "unsafe cursor", prefix: "media/", cursor: "bad\ncursor", limit: 1},
		{name: "zero limit", prefix: "media/", limit: 0},
		{name: "excessive limit", prefix: "media/", limit: 1001},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.List(context.Background(), tt.prefix, tt.cursor, tt.limit)
			if err == nil || err.Error() != ErrorInvalidArgument {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := client.Delete(context.Background(), "media/../secret"); err == nil || err.Error() != ErrorInvalidArgument {
		t.Fatalf("delete error = %v", err)
	}
	policies, requests := recorder.snapshot()
	if len(policies) != 0 || len(requests) != 0 {
		t.Fatalf("invalid inputs reached network: policies=%d requests=%d", len(policies), len(requests))
	}
}

func TestOSSOperationsHonorCanceledContextBeforeNetwork(t *testing.T) {
	client, recorder := newProtocolTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("OSS must not be called for canceled context")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Upload(ctx, "media/example.jpg", []byte("fixture"), "text/plain", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("upload error = %v", err)
	}
	if _, err := client.ResolvePrivate(ctx, "media/example.jpg", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve error = %v", err)
	}
	if _, _, _, _, err := client.Stat(ctx, "media/example.jpg"); !errors.Is(err, context.Canceled) {
		t.Fatalf("stat error = %v", err)
	}
	if _, err := client.List(ctx, "media/", "", 1); err == nil || err.Error() != ErrorList {
		t.Fatalf("list error = %v", err)
	}
	if err := client.Delete(ctx, "media/example.jpg"); err == nil || err.Error() != ErrorDelete {
		t.Fatalf("delete error = %v", err)
	}
	policies, requests := recorder.snapshot()
	if len(policies) != 0 || len(requests) != 0 {
		t.Fatalf("canceled operations reached network: policies=%d requests=%d", len(policies), len(requests))
	}
}

func TestListAndDeleteCancelInFlightOSSRequests(t *testing.T) {
	tests := []struct {
		name     string
		wantCode string
		call     func(context.Context, *Client) error
	}{
		{
			name:     "list",
			wantCode: ErrorList,
			call: func(ctx context.Context, client *Client) error {
				_, err := client.List(ctx, "media/", "", 1)
				return err
			},
		},
		{
			name:     "delete",
			wantCode: ErrorDelete,
			call: func(ctx context.Context, client *Client) error {
				return client.Delete(ctx, "media/example.jpg")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			requestCanceled := make(chan struct{})
			client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				close(started)
				select {
				case <-request.Context().Done():
					close(requestCanceled)
				case <-time.After(time.Second):
					http.Error(w, "request was not canceled", http.StatusGatewayTimeout)
				}
			}))
			ctx, cancel := context.WithCancel(context.Background())
			errCh := make(chan error, 1)
			go func() { errCh <- tt.call(ctx, client) }()
			select {
			case <-started:
			case err := <-errCh:
				t.Fatalf("operation ended before OSS request started: %v", err)
			case <-time.After(time.Second):
				t.Fatal("OSS request did not start")
			}
			cancel()
			select {
			case err := <-errCh:
				if err == nil || err.Error() != tt.wantCode {
					t.Fatalf("error = %v", err)
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("operation did not stop after context cancellation")
			}
			select {
			case <-requestCanceled:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("OSS HTTP request did not observe context cancellation")
			}
		})
	}
}

func TestUploadAndStatCancelInFlightOSSRequests(t *testing.T) {
	tests := []struct {
		name                 string
		observeServerContext bool
		call                 func(context.Context, *Client) error
	}{
		{
			name: "upload",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.Upload(ctx, "media/example.jpg", []byte("fixture"), "text/plain", true)
				return err
			},
		},
		{
			name:                 "stat",
			observeServerContext: true,
			call: func(ctx context.Context, client *Client) error {
				_, _, _, _, err := client.Stat(ctx, "media/example.jpg")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			requestCanceled := make(chan struct{})
			client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				close(started)
				select {
				case <-request.Context().Done():
					close(requestCanceled)
				case <-time.After(time.Second):
					http.Error(w, "request was not canceled", http.StatusGatewayTimeout)
				}
			}))
			ctx, cancel := context.WithCancel(context.Background())
			errCh := make(chan error, 1)
			go func() { errCh <- tt.call(ctx, client) }()
			select {
			case <-started:
			case err := <-errCh:
				t.Fatalf("operation ended before OSS request started: %v", err)
			case <-time.After(time.Second):
				t.Fatal("OSS request did not start")
			}
			cancel()
			select {
			case err := <-errCh:
				if err == nil {
					t.Fatal("operation unexpectedly succeeded")
				}
			case <-time.After(250 * time.Millisecond):
				t.Fatal("operation did not stop after context cancellation")
			}
			if tt.observeServerContext {
				select {
				case <-requestCanceled:
				case <-time.After(250 * time.Millisecond):
					t.Fatal("OSS HTTP request did not observe context cancellation")
				}
			}
		})
	}
}

func TestUploadCancelsMetadataReadWithoutHidingCommittedUpload(t *testing.T) {
	metadataStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPut {
			w.Header().Set("ETag", `"put-etag"`)
			return
		}
		if request.Method != http.MethodHead {
			t.Fatalf("method = %s", request.Method)
		}
		close(metadataStarted)
		select {
		case <-request.Context().Done():
			close(requestCanceled)
		case <-time.After(time.Second):
			http.Error(w, "request was not canceled", http.StatusGatewayTimeout)
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	type uploadOutcome struct {
		result UploadResult
		err    error
	}
	outcomeCh := make(chan uploadOutcome, 1)
	go func() {
		result, err := client.Upload(ctx, "media/example.jpg", []byte("fixture"), "text/plain", true)
		outcomeCh <- uploadOutcome{result: result, err: err}
	}()
	select {
	case <-metadataStarted:
	case <-time.After(time.Second):
		t.Fatal("metadata request did not start")
	}
	cancel()
	select {
	case outcome := <-outcomeCh:
		if outcome.err != nil {
			t.Fatalf("committed upload returned error: %v", outcome.err)
		}
		if outcome.result.Size != int64(len("fixture")) {
			t.Fatalf("fallback size = %d", outcome.result.Size)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("upload did not stop after metadata context cancellation")
	}
	select {
	case <-requestCanceled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("metadata HTTP request did not observe context cancellation")
	}
}

func TestNewClientConfiguresBoundedSTSTimeouts(t *testing.T) {
	client, err := NewClient(profile.Profile{
		Name:            "timeout-test",
		Bucket:          "component-bucket",
		Region:          "cn-test",
		RoleArn:         "acs:ram::123:role/test",
		AccessKeyID:     "TEST_ACCESS_KEY",
		AccessKeySecret: "TEST_ACCESS_SECRET",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.stsClient.ConnectTimeout == nil || *client.stsClient.ConnectTimeout != 5_000 {
		t.Fatalf("STS connect timeout = %v", client.stsClient.ConnectTimeout)
	}
	if client.stsClient.ReadTimeout == nil || *client.stsClient.ReadTimeout != 15_000 {
		t.Fatalf("STS read timeout = %v", client.stsClient.ReadTimeout)
	}
}

func TestSTSReadTimeoutBoundsBlockedEndpoint(t *testing.T) {
	started := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-requestCanceled
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	protocol := "HTTP"
	region := "cn-test"
	ak, sk := "TEST_ACCESS_KEY", "TEST_ACCESS_SECRET"
	connectTimeout, readTimeout := 50, 50
	endpoint := serverURL.Host
	stsClient, err := sts.NewClient(&openapi.Config{
		AccessKeyId:     &ak,
		AccessKeySecret: &sk,
		RegionId:        &region,
		Endpoint:        &endpoint,
		Protocol:        &protocol,
		ConnectTimeout:  &connectTimeout,
		ReadTimeout:     &readTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		prof: profile.Profile{
			Bucket:  "component-bucket",
			Region:  region,
			RoleArn: "acs:ram::123:role/test",
		},
		stsClient: stsClient,
	}

	start := time.Now()
	_, err = client.List(context.Background(), "media/", "", 1)
	close(requestCanceled)
	if err == nil || err.Error() != ErrorSTS {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("STS timeout took %s", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("STS endpoint was not reached")
	}
}

func TestDeleteClassifiesProviderAndIncompleteSTSErrorsSafely(t *testing.T) {
	providerCanary := "DELETE_PROVIDER_SECRET_CANARY"
	client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, `<Error><Code>AccessDenied</Code><Message>%s</Message></Error>`, providerCanary)
	}))
	err := client.Delete(context.Background(), "media/example.jpg")
	if err == nil || err.Error() != ErrorDelete || strings.Contains(fmt.Sprintf("%#v", err), providerCanary) {
		t.Fatalf("delete error = %#v", err)
	}

	stsCanary := "DELETE_STS_SECRET_CANARY"
	stsBody := `{"RequestId":"request-id","Credentials":{"SecurityToken":"` + stsCanary + `","AccessKeyId":"TEST_TEMP_ACCESS_KEY"}}`
	client, _ = newProtocolTestClientWithSTSBody(t, stsBody, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("OSS must not be called when STS credentials are incomplete")
	}))
	err = client.Delete(context.Background(), "media/example.jpg")
	if err == nil || err.Error() != ErrorSTS || strings.Contains(fmt.Sprintf("%#v", err), stsCanary) {
		t.Fatalf("STS error = %#v", err)
	}
}

func TestStatUsesDetailedMetadataForContentType(t *testing.T) {
	client, _ := newProtocolTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead {
			t.Fatalf("method = %s", request.Method)
		}
		if _, lightweight := request.URL.Query()["objectMeta"]; lightweight {
			w.Header().Set("Content-Length", "598")
			w.Header().Set("ETag", `"fixture-etag"`)
			return
		}
		w.Header().Set("Content-Length", "598")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("ETag", `"fixture-etag"`)
	}))

	exists, size, contentType, etag, err := client.Stat(context.Background(), "media/fixture.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || size != 598 || contentType != "text/plain" || etag != "fixture-etag" {
		t.Fatalf("stat = %v, %d, %q, %q", exists, size, contentType, etag)
	}
}
