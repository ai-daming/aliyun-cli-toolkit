package app

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newUploadCmd() *cobra.Command {
	var (
		profileName, key, file, contentType string
		private                             bool
	)
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a file to OSS and return key + url + etag",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			res, err := c.Upload(context.Background(), key, data, contentType, private)
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	cmd.Flags().StringVar(&file, "file", "", "Path to file to upload (required)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type header")
	cmd.Flags().BoolVar(&private, "private", false, "Store as private (no stable URL returned)")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
