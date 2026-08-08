// Package idcard verifies ID cards via Alibaba Cloud CloudAuth
// (Id2MetaVerifyWithOCR: OCR + two-factor name/ID check in one call).
// It consumes image URLs only; resolving private OSS keys to URLs is the
// caller's job (compose with aliyun-media-cli resolve).
package idcard

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
)

// percentEncode implements the aliyun RPC signature URL encoding:
// RFC3986 percent-encoding (via url.QueryEscape), then + -> %20,
// * -> %2A, %7E -> ~. Matches Java's percentEncode in
// AliyunId2MetaVerifyService.
func percentEncode(s string) (string, error) {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded, nil
}

// sign computes the HMAC-SHA1 signature for an aliyun RPC API request.
// method is "GET" or "POST". secret is the AccessKeySecret.
// Returns base64-encoded signature.
func sign(params map[string]string, method, secret string) (string, error) {
	// sort parameter names
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// build canonical query string: &k1=v1&k2=v2... (each k and v percent-encoded)
	var b strings.Builder
	for _, k := range keys {
		ek, err := percentEncode(k)
		if err != nil {
			return "", err
		}
		ev, err := percentEncode(params[k])
		if err != nil {
			return "", err
		}
		b.WriteString("&")
		b.WriteString(ek)
		b.WriteString("=")
		b.WriteString(ev)
	}
	canonical := b.String()
	// remove leading '&'
	canonical = canonical[1:]

	// string to sign: METHOD&%2F&<percentEncode(canonical)>
	pe, err := percentEncode(canonical)
	if err != nil {
		return "", err
	}
	stringToSign := method + "&%2F&" + pe

	// HMAC-SHA1 with key = secret + "&"
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
