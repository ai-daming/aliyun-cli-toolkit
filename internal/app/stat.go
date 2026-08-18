package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newStatCmd() *cobra.Command {
	var profileName, key string
	cmd := &cobra.Command{
		Use:   "stat",
		Short: "Report object metadata (exists/size/content-type/etag)",
		RunE: func(cmd *cobra.Command, args []string) error {
			validatedProfile, err := validateProfileName(profileName)
			if err != nil {
				return err
			}
			validatedKey, err := validateObjectKey(key)
			if err != nil {
				return err
			}
			p, err := profile.Load(validatedProfile)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			exists, size, ct, etag, err := c.Stat(cmd.Context(), validatedKey)
			if err != nil {
				return err
			}
			res := map[string]any{"exists": exists}
			if exists {
				res["size"] = size
				if ct != "" {
					res["contentType"] = ct
				}
				res["etag"] = etag
			}
			return output.PrintJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
