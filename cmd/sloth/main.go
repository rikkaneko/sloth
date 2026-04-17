package main

import (
	"context"
	"os"

	"sloth/internal/cli"
)

var Version = "dev"

func main() {
	app := cli.NewApp(Version)
	err := app.Run(context.Background(), os.Args[1:])
	cli.ExitWithError(err)
}
