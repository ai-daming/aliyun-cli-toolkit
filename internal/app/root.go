package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo"
)

// NewRootCmd assembles the full command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "aliyun-media-cli",
		Short:         "Stateless Alibaba Cloud OSS media CLI",
		Version:       buildinfo.Version,
		SilenceErrors: true, // main() emits the JSON error object
		SilenceUsage:  true, // usage dump is noise for JSON consumers
	}
	root.AddCommand(newProfileCmd())
	root.AddCommand(newUploadCmd())
	root.AddCommand(newResolveCmd())
	root.AddCommand(newStatCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newDeleteCmd())
	return root
}
