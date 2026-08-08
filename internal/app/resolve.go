package app

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newResolveCmd() *cobra.Command {
	var (
		profileName, key string
		ttl              time.Duration
		isPublic         bool
	)
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve an object key to a readable URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			c, err := oss.NewClient(p)
			if err != nil {
				return err
			}
			if isPublic {
				return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
					"url":        c.PublicURL(key),
					"visibility": "public",
				})
			}
			d := ttl
			if d == 0 {
				d = 15 * time.Minute
			}
			url, err := c.ResolvePrivate(context.Background(), key, d)
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"url":        url,
				"visibility": "private",
				"expiresAt":  time.Now().Add(d).UTC().Format(time.RFC3339),
			})
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "OSS object key (required)")
	cmd.Flags().DurationVar(&ttl, "ttl", 0, "Signed URL validity (default 15m)")
	cmd.Flags().BoolVar(&isPublic, "public", false, "Treat as public; return stable URL without signing")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
