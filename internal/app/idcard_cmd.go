package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/idcard"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

// idcard profiles store region/endpoint/AK/SK. They omit the OSS fields.
// We reuse profile.Profile (extra fields are empty, omitted via omitempty).

func idcardProfileAdd() *cobra.Command {
	var p profile.Profile
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new CloudAuth profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p.Name = args[0]
			if err := p.Save(); err != nil {
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]string{"profile": p.Name, "status": "saved"})
		},
	}
	cmd.Flags().StringVar(&p.Region, "region", "", "CloudAuth region (e.g. cn-shanghai)")
	cmd.Flags().StringVar(&p.Endpoint, "endpoint", "", "CloudAuth endpoint")
	cmd.Flags().StringVar(&p.AccessKeyID, "access-key-id", "", "AccessKey ID")
	cmd.Flags().StringVar(&p.AccessKeySecret, "access-key-secret", "", "AccessKey Secret")
	return cmd
}

func idcardProfileList() *cobra.Command {
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

func idcardProfileRemove() *cobra.Command {
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

func idcardProfileShow() *cobra.Command {
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

func newIdcardVerifyCmd() *cobra.Command {
	var (
		profileName, frontURL, backURL string
	)
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify an ID card (OCR + two-factor) via CloudAuth",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profile.Load(profileName)
			if err != nil {
				return err
			}
			v := idcard.NewVerifier(p)
			res, err := v.Verify(cmd.Context(), frontURL, backURL)
			if err != nil {
				// transport/HTTP error: emit the result (ok=false) AND signal
				// failure via non-zero exit by returning the error.
				_ = output.PrintJSON(cmd.OutOrStdout(), res)
				return err
			}
			return output.PrintJSON(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&frontURL, "front-url", "", "Readable URL of ID-card front image (required)")
	cmd.Flags().StringVar(&backURL, "back-url", "", "Readable URL of ID-card back image (optional)")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("front-url")
	return cmd
}
