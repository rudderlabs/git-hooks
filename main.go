package main

import (
	"log"
	"os"

	"github.com/lvrach/git-hooks/commands"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "git-hooks",
		Usage: "Manage and execute Git hooks",
		Commands: []*cli.Command{
			commands.Config,
			commands.Implode,
			commands.Hooks,
			commands.Add,
			commands.Remove,
			commands.CleanLocalHooks,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
