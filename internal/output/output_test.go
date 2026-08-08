package output

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]any{"key": "k", "n": 3}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("output must end with newline, got %q", got)
	}
	want := `{"key":"k","n":3}` + "\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintJSONSpecialChars(t *testing.T) {
	// error messages with quotes/slashes must be safely encoded
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]string{"msg": `bad "thing" \ path`}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `\"thing\"`) {
		t.Fatalf("quotes not escaped: %s", buf.String())
	}
}

func TestPrintError(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, "boom")
	got := buf.String()
	want := `{"error":"boom"}` + "\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintErrorSpecialChars(t *testing.T) {
	var buf bytes.Buffer
	PrintError(&buf, `failed: "x" <y>`)
	got := buf.String()
	if !strings.Contains(got, `\"x\"`) || !strings.Contains(got, `\u003cy`) {
		t.Fatalf("special chars not escaped: %s", got)
	}
}

func TestPrintJSONMarshalError(t *testing.T) {
	// channels and funcs cannot be JSON-marshaled
	var buf bytes.Buffer
	err := PrintJSON(&buf, func() {})
	if err == nil {
		t.Fatal("expected marshal error for non-serializable value")
	}
}

func TestExitWithError(t *testing.T) {
	// Exercise ExitWithError in a subprocess so os.Exit doesn't kill the test runner.
	if os.Getenv("OUTPUT_EXIT_TEST") == "1" {
		ExitWithError("kaboom")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestExitWithError")
	cmd.Env = append(os.Environ(), "OUTPUT_EXIT_TEST=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("not an ExitError: %T %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "kaboom") {
		t.Fatalf("stderr should contain error msg, got %q", stderr.String())
	}
}
