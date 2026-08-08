package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo"
)

// NewRootCmd assembles the full command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "aliyun-media-cli",
		Short:   "Stateless Alibaba Cloud OSS media CLI",
		Version: buildinfo.Version,
	}
	root.AddCommand(newProfileCmd())
	root.AddCommand(newUploadCmd())
	root.AddCommand(newResolveCmd())
	root.AddCommand(newStatCmd())
	return root
}
