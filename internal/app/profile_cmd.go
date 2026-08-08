package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage OSS credential profiles",
	}
	cmd.AddCommand(newProfileAddCmd(), newProfileListCmd(), newProfileRemoveCmd(), newProfileShowCmd())
	return cmd
}

func newProfileAddCmd() *cobra.Command {
	var p profile.Profile
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Name = args[0]
			if err := p.Save(); err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]string{"profile": p.Name, "status": "saved"})
		},
	}
	cmd.Flags().StringVar(&p.Bucket, "bucket", "", "OSS bucket name")
	cmd.Flags().StringVar(&p.Region, "region", "", "OSS region (e.g. cn-huhehaote)")
	cmd.Flags().StringVar(&p.Endpoint, "endpoint", "", "OSS endpoint")
	cmd.Flags().StringVar(&p.RoleArn, "role-arn", "", "RAM role ARN for STS AssumeRole")
	cmd.Flags().StringVar(&p.AccessKeyID, "access-key-id", "", "AccessKey ID")
	cmd.Flags().StringVar(&p.AccessKeySecret, "access-key-secret", "", "AccessKey Secret")
	cmd.Flags().StringVar(&p.PublicDomain, "public-domain", "", "Optional stable public/CDN domain")
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := profile.List()
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string][]string{"profiles": names})
		},
	}
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := profile.Delete(args[0]); err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]string{"profile": args[0], "status": "deleted"})
		},
	}
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a profile (masked)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(args[0])
			if err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), p.Masked())
		},
	}
}
