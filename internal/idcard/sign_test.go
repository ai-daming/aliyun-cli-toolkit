package idcard

import (
	"testing"
)

func TestPercentEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "abc"},
		{"a b", "a%20b"},
		{"a+b", "a%2Bb"},
		{"a*b", "a%2Ab"},
		{"~", "~"},
		{"a/b", "a%2Fb"},
		{"a=b", "a%3Db"},
		{"a&b", "a%26b"},
	}
	for _, c := range cases {
		got, err := percentEncode(c.in)
		if err != nil {
			t.Fatalf("percentEncode(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("percentEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSignDeterministic(t *testing.T) {
	params := map[string]string{
		"Action":           "Id2MetaVerifyWithOCR",
		"Version":          "2019-03-07",
		"AccessKeyId":      "TESTAK",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
	}
	sig, err := sign(params, "POST", "TESTSK")
	if err != nil {
		t.Fatal(err)
	}
	if sig == "" {
		t.Fatal("signature should be non-empty")
	}
	// determinism
	sig2, _ := sign(params, "POST", "TESTSK")
	if sig != sig2 {
		t.Fatalf("signature not deterministic: %q vs %q", sig, sig2)
	}
}

func TestSignMethodChangesOutput(t *testing.T) {
	params := map[string]string{"Action": "X", "AccessKeyId": "AK"}
	sigPost, _ := sign(params, "POST", "SK")
	sigGet, _ := sign(params, "GET", "SK")
	if sigPost == sigGet {
		t.Fatal("GET and POST should produce different signatures")
	}
}

func TestSignSecretChangesOutput(t *testing.T) {
	params := map[string]string{"Action": "X"}
	sig1, _ := sign(params, "POST", "SK1")
	sig2, _ := sign(params, "POST", "SK2")
	if sig1 == sig2 {
		t.Fatal("different secrets should produce different signatures")
	}
}

func TestSignParamsOrderInvariant(t *testing.T) {
	// building params in different insertion order must produce the same sig
	p1 := map[string]string{"Action": "X", "B": "2", "A": "1"}
	p2 := map[string]string{"A": "1", "B": "2", "Action": "X"}
	s1, _ := sign(p1, "POST", "SK")
	s2, _ := sign(p2, "POST", "SK")
	if s1 != s2 {
		t.Fatalf("order should not affect signature: %q vs %q", s1, s2)
	}
}
