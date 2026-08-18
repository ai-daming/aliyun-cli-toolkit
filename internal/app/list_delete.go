package app

import (
	"context"

	"github.com/spf13/cobra"

	mediaoss "github.com/mamamate/aliyun-cli-toolkit/internal/oss"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/mamamate/aliyun-cli-toolkit/internal/profile"
)

type mediaClient interface {
	List(context.Context, string, string, int) (mediaoss.ListResult, error)
	Delete(context.Context, string) error
}

type mediaClientFactory func(profile.Profile) (mediaClient, error)

func defaultMediaClientFactory(p profile.Profile) (mediaClient, error) {
	return mediaoss.NewClient(p)
}

func newListCmd() *cobra.Command {
	return newListCmdWithFactory(defaultMediaClientFactory)
}

func newListCmdWithFactory(factory mediaClientFactory) *cobra.Command {
	var profileName, prefix, limitValue, cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List OSS object metadata under an exact prefix",
		Args:  rejectPositionalArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			validatedProfile, err := validateProfileName(profileName)
			if err != nil {
				return err
			}
			validatedPrefix, err := validatePrefix(prefix)
			if err != nil {
				return err
			}
			limit, err := validateLimit(limitValue)
			if err != nil {
				return err
			}
			validatedCursor := ""
			if cmd.Flags().Changed("cursor") {
				validatedCursor, err = validateCursor(cursor)
				if err != nil {
					return err
				}
			}
			p, err := profile.Load(validatedProfile)
			if err != nil {
				return commandError("PROFILE_ERROR")
			}
			client, err := factory(p)
			if err != nil {
				return commandError("PROFILE_ERROR")
			}
			result, err := client.List(cmd.Context(), validatedPrefix, validatedCursor, limit)
			if err != nil {
				if code, ok := mediaoss.ErrorCode(err); ok {
					return commandError(code)
				}
				return commandError("OSS_LIST_ERROR")
			}
			if err := output.PrintJSON(cmd.OutOrStdout(), result); err != nil {
				return commandError("OUTPUT_ERROR")
			}
			return nil
		},
	}
	setSafeFlagErrors(cmd)
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Non-empty OSS object prefix (required)")
	cmd.Flags().StringVar(&limitValue, "limit", "", "Maximum objects to return, 1-1000 (required)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Opaque continuation cursor")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	return newDeleteCmdWithFactory(defaultMediaClientFactory)
}

func newDeleteCmdWithFactory(factory mediaClientFactory) *cobra.Command {
	var profileName, key string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Idempotently delete one exact OSS object key",
		Args:  rejectPositionalArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
				return commandError("PROFILE_ERROR")
			}
			client, err := factory(p)
			if err != nil {
				return commandError("PROFILE_ERROR")
			}
			if err := client.Delete(cmd.Context(), validatedKey); err != nil {
				if code, ok := mediaoss.ErrorCode(err); ok {
					return commandError(code)
				}
				return commandError("OSS_DELETE_ERROR")
			}
			result := struct {
				Key     string `json:"key"`
				Deleted bool   `json:"deleted"`
			}{Key: validatedKey, Deleted: true}
			if err := output.PrintJSON(cmd.OutOrStdout(), result); err != nil {
				return commandError("OUTPUT_ERROR")
			}
			return nil
		},
	}
	setSafeFlagErrors(cmd)
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name (required)")
	cmd.Flags().StringVar(&key, "key", "", "Exact OSS object key (required)")
	return cmd
}

func rejectPositionalArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return commandError("INVALID_ARGUMENT")
	}
	return nil
}

func setSafeFlagErrors(cmd *cobra.Command) {
	cmd.SetFlagErrorFunc(func(*cobra.Command, error) error {
		return commandError("INVALID_ARGUMENT")
	})
}
