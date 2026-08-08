package main

import (
	"os"

	"github.com/mamamate/aliyun-cli-toolkit/internal/app"
	"github.com/mamamate/aliyun-cli-toolkit/internal/output"
)

func main() {
	if err := app.NewRootCmd().Execute(); err != nil {
		output.ExitWithError(err.Error())
	}
	_ = os.Stdout
}
