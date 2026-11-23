package app

import (
	"errors"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
	"github.com/spf13/cobra"
)

func browserUploadClient(profileName, key string) (*oss.Client, string, error) {
	name, err := validateProfileName(profileName)
	if err != nil {
		return nil, "", err
	}
	key, err = validateObjectKey(key)
	if err != nil {
		return nil, "", err
	}
	p, err := profile.Load(name)
	if err != nil {
		return nil, "", err
	}
	c, err := oss.NewClient(p)
	return c, key, err
}

func newSignUploadCmd() *cobra.Command {
	var profileName, key, contentType string
	var size int64
	var private bool
	cmd := &cobra.Command{
		Use: "sign-upload", Short: "Sign a short-lived OSS form for one browser upload", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, validatedKey, err := browserUploadClient(profileName, key)
			if err != nil {
				return err
			}
			result, err := client.SignBrowserUpload(cmd.Context(), validatedKey, contentType, size, private)
			if err != nil {
				return errors.New("cannot sign OSS browser upload")
			}
			return output.PrintJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name")
	cmd.Flags().StringVar(&key, "key", "", "Exact object key")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Expected MIME type")
	cmd.Flags().Int64Var(&size, "size", 0, "Exact file length in bytes")
	cmd.Flags().BoolVar(&private, "private", false, "Store with private ACL")
	for _, name := range []string{"profile", "key", "content-type", "size"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

func newInspectUploadCmd() *cobra.Command {
	var profileName, key string
	cmd := &cobra.Command{
		Use: "inspect-upload", Short: "Read metadata and a bounded file header from OSS", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, validatedKey, err := browserUploadClient(profileName, key)
			if err != nil {
				return err
			}
			result, err := client.InspectBrowserUpload(cmd.Context(), validatedKey)
			if err != nil {
				return errors.New("cannot inspect OSS browser upload")
			}
			return output.PrintJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name")
	cmd.Flags().StringVar(&key, "key", "", "Exact object key")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
