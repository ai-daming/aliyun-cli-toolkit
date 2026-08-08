package app

import "github.com/spf13/cobra"

func newResolveCmd() *cobra.Command {
	return &cobra.Command{Use: "resolve", Short: "stub", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
