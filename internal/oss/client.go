// Package oss wraps the Alibaba Cloud OSS SDK + STS AssumeRole flow.
// Each operation receives temporary credentials scoped to its exact action and
// object key or list prefix; no consumer-specific storage semantics live here.
package oss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	sts "github.com/alibabacloud-go/sts-20150401/client"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/mamamate/aliyun-cli-toolkit/internal/media"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// Client performs OSS operations against the bucket configured in a Profile.
type Client struct {
	prof             profile.Profile
	stsClient        *sts.Client
	ossClientOptions []oss.ClientOption
}

// NewClient builds an OSS client from a profile.
func NewClient(p profile.Profile) (*Client, error) {
	if strings.TrimSpace(p.Region) == "" {
		return nil, fmt.Errorf("profile %q: region is required (e.g. cn-huhehaote); set it via --region or region= in the profile", p.Name)
	}
	if err := media.ValidateBucket(p.Bucket); err != nil {
		return nil, fmt.Errorf("profile %q: valid bucket is required", p.Name)
	}
	if strings.TrimSpace(p.RoleArn) == "" {
		return nil, fmt.Errorf("profile %q: role ARN is required", p.Name)
	}
	if strings.TrimSpace(p.AccessKeyID) == "" {
		return nil, fmt.Errorf("profile %q: access key ID is required", p.Name)
	}
	if strings.TrimSpace(p.AccessKeySecret) == "" {
		return nil, fmt.Errorf("profile %q: access key secret is required", p.Name)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Endpoint)), "http://") {
		return nil, fmt.Errorf("profile %q: endpoint must use HTTPS", p.Name)
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

// assumeRole obtains STS temporary credentials scoped by a serialized policy.
func (c *Client) assumeRole(policy string) (*stsCreds, error) {
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
	if resp.Body == nil || resp.Body.Credentials == nil ||
		resp.Body.Credentials.AccessKeyId == nil ||
		resp.Body.Credentials.AccessKeySecret == nil ||
		resp.Body.Credentials.SecurityToken == nil {
		return nil, fmt.Errorf("STS returned incomplete credentials")
	}
	cr := resp.Body.Credentials
	return &stsCreds{
		ak:    *cr.AccessKeyId,
		sk:    *cr.AccessKeySecret,
		token: *cr.SecurityToken,
	}, nil
}

func (c *Client) assumeObjectRole(action, objectKey string) (*stsCreds, error) {
	policy, err := buildObjectPolicy(c.prof.Bucket, action, objectKey)
	if err != nil {
		return nil, err
	}
	return c.assumeRole(policy)
}

func (c *Client) assumeListRole(prefix string) (*stsCreds, error) {
	policy, err := buildListPolicy(c.prof.Bucket, prefix)
	if err != nil {
		return nil, err
	}
	return c.assumeRole(policy)
}

func (c *Client) ossEndpoint() string {
	if c.prof.Endpoint != "" {
		return c.prof.Endpoint
	}
	return fmt.Sprintf("oss-%s.aliyuncs.com", c.prof.Region)
}

func newOSSClient(endpoint, ak, sk, token string, options ...oss.ClientOption) (*oss.Client, error) {
	// Force HTTPS: the SDK picks scheme from the endpoint prefix, defaulting to
	// HTTP when bare. Signed URLs and public URLs must use HTTPS so that STS
	// security tokens and private media never travel over plaintext.
	ep := endpoint
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		ep = "https://" + ep
	}
	options = append([]oss.ClientOption{oss.SecurityToken(token)}, options...)
	return oss.New(ep, ak, sk, options...)
}

// Upload puts an object into OSS. When private is true, the object ACL is set
// to private and no stable URL is returned.
func (c *Client) Upload(ctx context.Context, key string, data []byte, contentType string, private bool) (UploadResult, error) {
	creds, err := c.assumeObjectRole("oss:PutObject", key)
	if err != nil {
		return UploadResult{}, err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
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
	creds, err := c.assumeObjectRole("oss:GetObject", key)
	if err != nil {
		return "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
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
	creds, err := c.assumeObjectRole("oss:GetObject", key)
	if err != nil {
		return false, 0, "", "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
	if err != nil {
		return false, 0, "", "", err
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return false, 0, "", "", err
	}
	headers, err := bucket.GetObjectDetailedMeta(key)
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
	creds, err := c.assumeObjectRole("oss:GetObject", key)
	if err != nil {
		return 0, "", err
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
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

// ObjectSummary is one item returned by List.
type ObjectSummary struct {
	Key          string `json:"key"`
	LastModified string `json:"lastModified"`
	Size         int64  `json:"size"`
	ETag         string `json:"etag"`
}

// ListResult is one page of object metadata. A nil cursor means traversal ended.
type ListResult struct {
	Objects    []ObjectSummary `json:"objects"`
	NextCursor *string         `json:"nextCursor"`
}

// List returns one ListObjectsV2 page under an exact prefix. The context
// cancels the OSS request after temporary credentials are obtained; the STS
// SDK does not accept a context, so its in-flight call remains timeout-bound.
func (c *Client) List(ctx context.Context, prefix, cursor string, limit int) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, newOperationError(ErrorList, err)
	}
	if err := media.ValidateObjectInput(prefix); err != nil {
		return ListResult{}, newOperationError(ErrorInvalidArgument, err)
	}
	if cursor != "" {
		if err := media.ValidateCursor(cursor); err != nil {
			return ListResult{}, newOperationError(ErrorInvalidArgument, err)
		}
	}
	if err := media.ValidateLimit(limit); err != nil {
		return ListResult{}, newOperationError(ErrorInvalidArgument, err)
	}
	creds, err := c.assumeListRole(prefix)
	if err != nil {
		return ListResult{}, newOperationError(ErrorSTS, err)
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
	if err != nil {
		return ListResult{}, newOperationError(ErrorList, err)
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return ListResult{}, newOperationError(ErrorList, err)
	}
	options := []oss.Option{oss.Prefix(prefix), oss.MaxKeys(limit), oss.WithContext(ctx)}
	if cursor != "" {
		options = append(options, oss.ContinuationToken(cursor))
	}
	page, err := bucket.ListObjectsV2(options...)
	if err != nil {
		return ListResult{}, classifyListError(err)
	}
	if len(page.Objects) > limit || page.Prefix != prefix || page.ContinuationToken != cursor {
		return ListResult{}, newOperationError(ErrorInvalidResponse, nil)
	}

	objects := make([]ObjectSummary, 0, len(page.Objects))
	for _, object := range page.Objects {
		etag := strings.Trim(object.ETag, `"`)
		if !strings.HasPrefix(object.Key, prefix) || object.Size < 0 ||
			object.Key == "" || object.LastModified.IsZero() || etag == "" {
			return ListResult{}, newOperationError(ErrorInvalidResponse, nil)
		}
		objects = append(objects, ObjectSummary{
			Key:          object.Key,
			LastModified: object.LastModified.UTC().Format(time.RFC3339),
			Size:         object.Size,
			ETag:         etag,
		})
	}

	var nextCursor *string
	if page.IsTruncated {
		next := page.NextContinuationToken
		if next == cursor || media.ValidateCursor(next) != nil {
			return ListResult{}, newOperationError(ErrorInvalidResponse, nil)
		}
		nextCursor = &next
	}
	return ListResult{Objects: objects, NextCursor: nextCursor}, nil
}

func classifyListError(err error) error {
	var serviceErr oss.ServiceError
	var transportErr *url.Error
	if errors.As(err, &serviceErr) || errors.As(err, &transportErr) {
		return newOperationError(ErrorList, err)
	}
	return newOperationError(ErrorInvalidResponse, err)
}

// Delete idempotently makes the object absent from the current read view. The
// context cancels the OSS request after temporary credentials are obtained;
// the STS SDK does not accept a context, so its in-flight call remains
// timeout-bound. Versioned buckets may retain historical versions and markers.
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return newOperationError(ErrorDelete, err)
	}
	if err := media.ValidateObjectInput(key); err != nil {
		return newOperationError(ErrorInvalidArgument, err)
	}
	creds, err := c.assumeObjectRole("oss:DeleteObject", key)
	if err != nil {
		return newOperationError(ErrorSTS, err)
	}
	ossClient, err := newOSSClient(c.ossEndpoint(), creds.ak, creds.sk, creds.token, c.ossClientOptions...)
	if err != nil {
		return newOperationError(ErrorDelete, err)
	}
	bucket, err := ossClient.Bucket(c.prof.Bucket)
	if err != nil {
		return newOperationError(ErrorDelete, err)
	}
	if err := bucket.DeleteObject(key, oss.WithContext(ctx)); err != nil {
		return newOperationError(ErrorDelete, err)
	}
	return nil
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
