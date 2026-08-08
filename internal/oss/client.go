// Package oss wraps the Alibaba Cloud OSS SDK + STS AssumeRole flow.
// It mirrors Mamamate's OssStsService: per-operation STS credentials
// scoped to a single object/action, then the OSS operation.
package oss

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	sts "github.com/alibabacloud-go/sts-20150401/client"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// Client performs OSS operations against the bucket configured in a Profile.
type Client struct {
	prof      profile.Profile
	stsClient *sts.Client
}

// NewClient builds an OSS client from a profile.
func NewClient(p profile.Profile) (*Client, error) {
	if p.Region == "" {
		return nil, fmt.Errorf("profile %q: region is required (e.g. cn-huhehaote); set it via --region or region= in the profile", p.Name)
	}
	region := p.Region
	ak, sk := p.AccessKeyID, p.AccessKeySecret
	cfg := &openapi.Config{
		AccessKeyId:     &ak,
		AccessKeySecret: &sk,
		RegionId:        &region,
	}
	stsClient, err := sts.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("init STS client: %w", err)
	}
	return &Client{prof: p, stsClient: stsClient}, nil
}

// UploadResult is returned by Upload.
type UploadResult struct {
	Key        string `json:"key"`
	URL        string `json:"url,omitempty"`
	Visibility string `json:"visibility"`
	Size       int64  `json:"size"`
	ETag       string `json:"etag"`
}

// stsCreds holds temporary credentials returned by AssumeRole.
type stsCreds struct {
	ak    string
	sk    string
	token string
}

// assumeRole obtains STS temporary credentials scoped to a single object+action.
func (c *Client) assumeRole(action, objectKey string) (*stsCreds, error) {
	policy := fmt.Sprintf(`{
  "Version": "1",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["%s"],
    "Resource": ["acs:oss:*:*:%s/%s"]
  }]}`, action, c.prof.Bucket, objectKey)

	session := "aliyun-media-cli"
	dur := int64(3600)
	req := &sts.AssumeRoleRequest{
		RoleArn:         &c.prof.RoleArn,
		RoleSessionName: &session,
		DurationSeconds: &dur,
		Policy:          &policy,
	}
	resp, err := c.stsClient.AssumeRole(req)
	if err != nil {
		return nil, fmt.Errorf("STS AssumeRole: %w", err)
	}
	if resp.Body == nil || resp.Body.Credentials == nil {
		return nil, fmt.Errorf("STS returned no credentials")
	}
	cr := resp.Body.Credentials
	return &stsCreds{
		ak:    *cr.AccessKeyId,
		sk:    *cr.AccessKeySecret,
		token: *cr.SecurityToken,
	}, nil
}

func (c *Client) ossEndpoint() string {
	if c.prof.Endpoint != "" {
		return c.prof.Endpoint
	}
	return fmt.Sprintf("oss-%s.aliyuncs.com", c.prof.Region)
}

func newOSSClient(endpoint, ak, sk, token string) (*oss.Client, error) {
	// Force HTTPS: the SDK picks scheme from the endpoint prefix, defaulting to
	// HTTP when bare. Signed URLs and public URLs must use HTTPS so that STS
	// security tokens and private media never travel over plaintext.
	ep := endpoint
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		ep = "https://" + ep
	}
	return oss.New(ep, ak, sk, oss.SecurityToken(token))
}

// Upload puts an object into OSS. When private is true, the object ACL is set
// to private and no stable URL is returned.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string, private bool) (UploadResult, error) {
	creds, err := c.assumeRole("oss:PutObject", key)
	if err != nil {
		return UploadResult{}, err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token)
	if err != nil {
		return UploadResult{}, fmt.Errorf("oss new: %w", err)
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return UploadResult{}, fmt.Errorf("oss bucket: %w", err)
	}

	opts := []oss.Option{oss.ContentLength(int64(len(data)))}
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}
	visibility := "public"
	if private {
		opts = append(opts, oss.ObjectACL(oss.ACLPrivate))
		visibility = "private"
	}

	if err := bucket.PutObject(key, bytes.NewReader(data), opts...); err != nil {
		return UploadResult{}, fmt.Errorf("oss put: %w", err)
	}

	// read back etag + size via HEAD. HEAD requires oss:GetObject permission, so
	// assume a separate GetObject-scoped credential rather than reusing the PutObject one.
	size, etag, herr := c.headAfterUpload(key)
	if herr != nil {
		size = int64(len(data))
		etag = ""
	}

	res := UploadResult{
		Key:        key,
		Visibility: visibility,
		Size:       size,
		ETag:       etag,
	}
	if !private {
		res.URL = c.PublicURL(key)
	}
	return res, nil
}

// ResolvePrivate generates a short-lived presigned GET URL for a private object.
func (c *Client) ResolvePrivate(ctx context.Context, key string, ttl time.Duration) (string, error) {
	creds, err := c.assumeRole("oss:GetObject", key)
	if err != nil {
		return "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token)
	if err != nil {
		return "", err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return "", err
	}
	url, err := bucket.SignURL(key, oss.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", fmt.Errorf("sign url: %w", err)
	}
	return url, nil
}

// PublicURL returns the stable public URL for an object.
func (c *Client) PublicURL(key string) string {
	if c.prof.PublicDomain != "" {
		return fmt.Sprintf("https://%s/%s", c.prof.PublicDomain, key)
	}
	return fmt.Sprintf("https://%s.%s/%s", c.prof.Bucket, c.ossEndpoint(), key)
}

// Stat reports object metadata. exists is false (not an error) when absent.
func (c *Client) Stat(ctx context.Context, key string) (exists bool, size int64, contentType, etag string, err error) {
	creds, err := c.assumeRole("oss:GetObject", key)
	if err != nil {
		return false, 0, "", "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token)
	if err != nil {
		return false, 0, "", "", err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return false, 0, "", "", err
	}
	headers, err := bucket.GetObjectMeta(key)
	if err != nil {
		if isNotFound(err) {
			return false, 0, "", "", nil
		}
		return false, 0, "", "", fmt.Errorf("oss head: %w", err)
	}
	size, _ = strconv.ParseInt(headers.Get("Content-Length"), 10, 64)
	return true, size, headers.Get("Content-Type"), strings.Trim(headers.Get("Etag"), `"`), nil
}

// headAfterUpload assumes GetObject-scoped creds and reads back size + etag.
func (c *Client) headAfterUpload(key string) (int64, string, error) {
	creds, err := c.assumeRole("oss:GetObject", key)
	if err != nil {
		return 0, "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token)
	if err != nil {
		return 0, "", err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return 0, "", err
	}
	headers, err := bucket.GetObjectMeta(key)
	if err != nil {
		return 0, "", err
	}
	size, _ := strconv.ParseInt(headers.Get("Content-Length"), 10, 64)
	return size, strings.Trim(headers.Get("Etag"), `"`), nil
}

// Delete removes an object. Used by tests for cleanup.
func (c *Client) Delete(ctx context.Context, key string) error {
	creds, err := c.assumeRole("oss:DeleteObject", key)
	if err != nil {
		return err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token)
	if err != nil {
		return err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return err
	}
	return bucket.DeleteObject(key)
}

func isNotFound(err error) bool {
	if ossErr, ok := err.(oss.ServiceError); ok {
		if ossErr.StatusCode == 404 {
			return true
		}
		code := ossErr.Code
		return strings.Contains(code, "NoSuch") || strings.Contains(code, "NotFound")
	}
	return strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404")
}
