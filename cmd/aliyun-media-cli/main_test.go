package main

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecuteContextReachesCobraCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	want := errors.New("context reached command")
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cancel()
			if !errors.Is(cmd.Context().Err(), context.Canceled) {
				t.Fatalf("command context error = %v", cmd.Context().Err())
			}
			return want
		},
	}

	if err := executeContext(ctx, cmd); !errors.Is(err, want) {
		t.Fatalf("executeContext error = %v", err)
	}
}
