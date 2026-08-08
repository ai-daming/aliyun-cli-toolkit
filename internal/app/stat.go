package app

import "github.com/spf13/cobra"

func newStatCmd() *cobra.Command {
	return &cobra.Command{Use: "stat", Short: "stub", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
