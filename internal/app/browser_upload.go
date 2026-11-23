package app

import (
	"context"

	"github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
	"github.com/spf13/cobra"
)

type browserUploadClient interface {
	SignBrowserUpload(context.Context, string, string, int64, bool) (oss.UploadForm, error)
	InspectBrowserUpload(context.Context, string) (oss.UploadedHeader, error)
}

type browserUploadClientFactory func(profile.Profile) (browserUploadClient, error)

func defaultBrowserUploadClient(p profile.Profile) (browserUploadClient, error) {
	return oss.NewClient(p)
}

func loadBrowserUploadClient(profileName, key string, factory browserUploadClientFactory) (browserUploadClient, string, error) {
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
		return nil, "", commandError("PROFILE_ERROR")
	}
	client, err := factory(p)
	if err != nil {
		return nil, "", commandError("PROFILE_ERROR")
	}
	return client, key, nil
}

func newSignUploadCmd() *cobra.Command {
	return newSignUploadCmdWithFactory(defaultBrowserUploadClient)
}

func newSignUploadCmdWithFactory(factory browserUploadClientFactory) *cobra.Command {
	var profileName, key, contentType string
	var size int64
	var private bool
	cmd := &cobra.Command{
		Use: "sign-upload", Short: "Sign a short-lived OSS form for one browser upload", Args: rejectPositionalArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, validatedKey, err := loadBrowserUploadClient(profileName, key, factory)
			if err != nil {
				return err
			}
			result, err := client.SignBrowserUpload(cmd.Context(), validatedKey, contentType, size, private)
			if err != nil {
				return commandError("OSS_SIGN_UPLOAD_ERROR")
			}
			if err := output.PrintJSON(cmd.OutOrStdout(), result); err != nil {
				return commandError("OUTPUT_ERROR")
			}
			return nil
		},
	}
	setSafeFlagErrors(cmd)
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
	return newInspectUploadCmdWithFactory(defaultBrowserUploadClient)
}

func newInspectUploadCmdWithFactory(factory browserUploadClientFactory) *cobra.Command {
	var profileName, key string
	cmd := &cobra.Command{
		Use: "inspect-upload", Short: "Read metadata and a bounded file header from OSS", Args: rejectPositionalArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, validatedKey, err := loadBrowserUploadClient(profileName, key, factory)
			if err != nil {
				return err
			}
			result, err := client.InspectBrowserUpload(cmd.Context(), validatedKey)
			if err != nil {
				return commandError("OSS_INSPECT_UPLOAD_ERROR")
			}
			if err := output.PrintJSON(cmd.OutOrStdout(), result); err != nil {
				return commandError("OUTPUT_ERROR")
			}
			return nil
		},
	}
	setSafeFlagErrors(cmd)
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name")
	cmd.Flags().StringVar(&key, "key", "", "Exact object key")
	_ = cmd.MarkFlagRequired("profile")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}
