package main

import (
	"fmt"
	"os"

	"github.com/redscaresu/infrafactory/internal/cli"
)

func main() {
	if err := cli.NewRootCmd(cli.WithUIAssets(uiAssets)).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCodeForError(err))
	}
}
