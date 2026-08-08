package app

import "github.com/spf13/cobra"

func newUploadCmd() *cobra.Command {
	return &cobra.Command{Use: "upload", Short: "stub", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
