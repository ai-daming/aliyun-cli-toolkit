package oss

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"

	sdk "github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/media"
)

const maxBrowserUploadBytes = 20 * 1024 * 1024

type UploadForm struct {
	URL       string            `json:"url"`
	Fields    map[string]string `json:"fields"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

func (c *Client) SignBrowserUpload(ctx context.Context, key, contentType string, size int64, private bool) (UploadForm, error) {
	if err := ctx.Err(); err != nil {
		return UploadForm{}, err
	}
	if err := validateBrowserUpload(key, contentType, size); err != nil {
		return UploadForm{}, err
	}
	acl := "default"
	if private {
		acl = "private"
	}
	policy, err := buildScopedObjectPolicy(c.prof.Bucket, "oss:PutObject", key, &policyCondition{
		StringEquals: map[string]string{"oss:x-oss-object-acl": acl},
	})
	if err != nil {
		return UploadForm{}, err
	}
	creds, err := c.assumeRole(policy)
	if err != nil {
		return UploadForm{}, err
	}
	if err := ctx.Err(); err != nil {
		return UploadForm{}, err
	}
	domain := c.prof.PublicDomain
	if domain == "" {
		domain = fmt.Sprintf("%s.oss-%s.aliyuncs.com", c.prof.Bucket, c.prof.Region)
	}
	return buildUploadForm(c.prof.Bucket, c.prof.Region, "https://"+domain, key, contentType, acl, size, time.Now(), *creds)
}

func validateBrowserUpload(key, contentType string, size int64) error {
	if media.ValidateObjectInput(key) != nil || size < 1 || size > maxBrowserUploadBytes {
		return fmt.Errorf("invalid upload key or size")
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/webp", "application/pdf":
		return nil
	default:
		return fmt.Errorf("unsupported upload content type")
	}
}

func buildUploadForm(bucket, region, endpoint, key, contentType, acl string, size int64, now time.Time, creds stsCreds) (UploadForm, error) {
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil ||
		target.Path != "" || target.RawQuery != "" || target.Fragment != "" {
		return UploadForm{}, fmt.Errorf("invalid browser upload endpoint")
	}
	expiry := now.UTC().Truncate(time.Second).Add(10 * time.Minute)
	date := now.UTC().Format("20060102")
	scope := date + "/" + region + "/oss/aliyun_v4_request"
	fields := map[string]string{
		"key":                     key,
		"x-oss-content-type":      contentType,
		"x-oss-object-acl":        acl,
		"x-oss-forbid-overwrite":  "true",
		"success_action_status":   "204",
		"x-oss-credential":        creds.ak + "/" + scope,
		"x-oss-security-token":    creds.token,
		"x-oss-signature-version": "OSS4-HMAC-SHA256",
		"x-oss-date":              now.UTC().Format("20060102T150405Z"),
	}
	conditions := []any{
		map[string]string{"bucket": bucket},
		[]any{"content-length-range", size, size},
	}
	for _, name := range []string{"key", "x-oss-content-type", "x-oss-object-acl", "x-oss-forbid-overwrite", "success_action_status"} {
		conditions = append(conditions, []string{"eq", "$" + name, fields[name]})
	}
	for _, name := range []string{"x-oss-credential", "x-oss-security-token", "x-oss-signature-version", "x-oss-date"} {
		conditions = append(conditions, map[string]string{name: fields[name]})
	}
	policy, err := json.Marshal(struct {
		Expiration time.Time `json:"expiration"`
		Conditions []any     `json:"conditions"`
	}{expiry, conditions})
	if err != nil {
		return UploadForm{}, err
	}
	fields["policy"] = base64.StdEncoding.EncodeToString(policy)
	signingKey := signHMAC([]byte("aliyun_v4"+creds.sk), date)
	for _, part := range []string{region, "oss", "aliyun_v4_request"} {
		signingKey = signHMAC(signingKey, part)
	}
	fields["x-oss-signature"] = hex.EncodeToString(signHMAC(signingKey, fields["policy"]))
	return UploadForm{URL: target.String(), Fields: fields, ExpiresAt: expiry}, nil
}

func signHMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

type UploadedHeader struct {
	Exists      bool   `json:"exists"`
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Header      []byte `json:"header,omitempty"`
}

// InspectBrowserUpload reads metadata and at most 32 bytes from the same object version.
func (c *Client) InspectBrowserUpload(ctx context.Context, key string) (UploadedHeader, error) {
	if err := ctx.Err(); err != nil {
		return UploadedHeader{}, err
	}
	creds, err := c.assumeObjectRole("oss:GetObject", key)
	if err != nil {
		return UploadedHeader{}, err
	}
	client, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
	if err != nil {
		return UploadedHeader{}, err
	}
	bucket, err := client.Bucket(c.prof.Bucket)
	if err != nil {
		return UploadedHeader{}, err
	}
	meta, err := bucket.GetObjectDetailedMeta(key, sdk.WithContext(ctx))
	if err != nil {
		if isNotFound(err) {
			return UploadedHeader{}, nil
		}
		return UploadedHeader{}, err
	}
	size, err := strconv.ParseInt(meta.Get("Content-Length"), 10, 64)
	if err != nil || size < 1 || meta.Get("ETag") == "" {
		return UploadedHeader{}, fmt.Errorf("invalid uploaded object metadata")
	}
	body, err := bucket.GetObject(key, sdk.Range(0, min(31, size-1)), sdk.IfMatch(meta.Get("ETag")), sdk.WithContext(ctx))
	if err != nil {
		return UploadedHeader{}, err
	}
	defer body.Close()
	prefix, err := io.ReadAll(io.LimitReader(body, 32))
	if err != nil {
		return UploadedHeader{}, err
	}
	return UploadedHeader{Exists: true, Size: size, ContentType: meta.Get("Content-Type"), Header: prefix}, nil
}
