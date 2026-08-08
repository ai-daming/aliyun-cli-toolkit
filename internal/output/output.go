// Package output provides JSON stdout/stderr helpers for the CLI.
// Contract: stdout is always valid compact JSON terminated by a newline.
// Errors go to stderr as {"error":"..."} with a non-zero exit code.
package output

import (
	"encoding/json"
	"io"
	"os"
)

// PrintJSON marshals v to compact JSON and writes it followed by a newline.
func PrintJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = w.Write([]byte("\n"))
	return err
}

// PrintError writes a JSON error object to w.
func PrintError(w io.Writer, msg string) {
	b, _ := json.Marshal(msg)
	_, _ = io.WriteString(w, `{"error":`+string(b)+`}`+"\n")
}

// ExitWithError prints an error to stderr and exits with code 1.
func ExitWithError(msg string) {
	PrintError(os.Stderr, msg)
	os.Exit(1)
}
