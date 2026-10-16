package main

import (
	"os"

	"github.com/redscaresu/infrafactory/internal/cli"
)

func main() {
	os.Exit(cli.ReportError(cli.NewRootCmd(cli.WithUIAssets(uiAssets)).Execute(), os.Stderr))
}
