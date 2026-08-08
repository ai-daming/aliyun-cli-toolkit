package app

import (
	"github.com/spf13/cobra"

	"github.com/mamamate/aliyun-cli-toolkit/internal/buildinfo"
)

// NewIdcardRootCmd assembles the command tree for aliyun-idcard-cli.
func NewIdcardRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "aliyun-idcard-cli",
		Short:         "Stateless Alibaba Cloud CloudAuth ID-card verification CLI",
		Version:       buildinfo.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(newIdcardProfileCmd(), newIdcardVerifyCmd())
	return root
}

// newIdcardProfileCmd wires profile management under the idcard binary.
// It reuses the same profile package (files, perms, masking) but is a
// separate command tree so the idcard binary is self-contained.
func newIdcardProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage CloudAuth credential profiles",
	}
	cmd.AddCommand(idcardProfileAdd(), idcardProfileList(), idcardProfileRemove(), idcardProfileShow())
	return cmd
}
