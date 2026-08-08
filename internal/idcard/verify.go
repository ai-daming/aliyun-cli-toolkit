package idcard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// Result holds the outcome of an Id2MetaVerifyWithOCR call.
type Result struct {
	OK            bool           `json:"ok"`
	Passed        bool           `json:"passed"`
	Name          string         `json:"name,omitempty"`
	IDCard        string         `json:"idCard,omitempty"`
	Gender        string         `json:"gender,omitempty"`
	Ethnicity     string         `json:"ethnicity,omitempty"`
	BirthDate     string         `json:"birthDate,omitempty"`
	Address       string         `json:"address,omitempty"`
	RequestID     string         `json:"requestId,omitempty"`
	ErrorMessage  string         `json:"errorMessage,omitempty"`
	VerifyMessage string         `json:"verifyMessage,omitempty"`
	Raw           map[string]any `json:"raw,omitempty"`
}

// Verifier calls the CloudAuth Id2MetaVerifyWithOCR endpoint.
type Verifier struct {
	profile profile.Profile
	client  *http.Client
}

// NewVerifier builds a verifier from a profile.
func NewVerifier(p profile.Profile) *Verifier {
	return &Verifier{
		profile: p,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Verify performs OCR + two-factor verification. frontURL is required,
// backURL is optional. ok=false means the API call itself failed.
// ok=true, passed=false means the call succeeded but name/ID didn't match.
func (v *Verifier) Verify(ctx context.Context, frontURL, backURL string) (Result, error) {
	endpoint := v.profile.Endpoint
	if endpoint == "" {
		endpoint = "cloudauth.cn-shanghai.aliyuncs.com"
	}
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}

	params := map[string]string{
		"Action":           "Id2MetaVerifyWithOCR",
		"Version":          "2019-03-07",
		"Format":           "JSON",
		"RegionId":         regionFromProfile(v.profile),
		"AccessKeyId":      v.profile.AccessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureNonce":   nonce(),
		"CertUrl":          frontURL,
	}
	if backURL != "" {
		params["CertNationalUrl"] = backURL
	}

	sig, err := sign(params, "POST", v.profile.AccessKeySecret)
	if err != nil {
		return Result{OK: false, ErrorMessage: "signing failed"}, err
	}
	params["Signature"] = sig

	form := encodeForm(params)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form))
	if err != nil {
		return Result{OK: false, ErrorMessage: err.Error()}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return Result{OK: false, ErrorMessage: err.Error()}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
		return Result{OK: false, ErrorMessage: msg}, fmt.Errorf("cloudauth returned %d", resp.StatusCode)
	}

	return parseResponse(body)
}

func regionFromProfile(p profile.Profile) string {
	if p.Region != "" {
		return p.Region
	}
	return "cn-shanghai"
}

func nonce() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func encodeForm(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("&")
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteString("=")
		b.WriteString(url.QueryEscape(params[k]))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "..."
}

// parseResponse turns the CloudAuth JSON response into a Result.
func parseResponse(body []byte) (Result, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return Result{OK: false, ErrorMessage: "parse error: " + err.Error()}, err
	}

	code := str(root["Code"])
	if code != "" && code != "200" {
		return Result{OK: false, ErrorMessage: "API error: " + str(root["Message"])}, nil
	}

	ro, _ := root["ResultObject"].(map[string]any)
	bizCode := str(ro["BizCode"])
	passed := bizCode == "1"

	r := Result{
		OK:        true,
		Passed:    passed,
		RequestID: str(root["RequestId"]),
		Raw:       root,
	}

	// CardInfo is a JSON string needing a second parse
	if cardStr := str(ro["CardInfo"]); cardStr != "" {
		var card map[string]any
		if err := json.Unmarshal([]byte(cardStr), &card); err == nil {
			r.Name = str(card["certName"])
			r.IDCard = str(card["certNo"])
			r.Ethnicity = str(card["nationality"])
			r.BirthDate = str(card["birthDate"])
			r.Address = str(card["address"])
		}
	}

	if !passed {
		if bizCode == "2" {
			r.VerifyMessage = "姓名与身份证号不匹配"
		} else if bizCode != "" {
			r.VerifyMessage = "核验结果: BizCode=" + bizCode
		}
	}

	return r, nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
