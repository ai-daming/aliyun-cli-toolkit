package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mamamate/aliyun-cli-toolkit/internal/app"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
	"github.com/spf13/cobra"
)

func executeContext(ctx context.Context, cmd *cobra.Command) error {
	return cmd.ExecuteContext(ctx)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := executeContext(ctx, app.NewRootCmd()); err != nil {
		output.ExitWithError(err.Error())
	}
}
