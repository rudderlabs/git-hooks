package main

import (
	"log"
	"os"

	"github.com/lvrach/git-hooks/commands"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "git-hooks-manager",
		Usage: "Manage and execute Git hooks",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "configure",
				Usage: "Configure Git to use this tool for hooks",
			},
		},
		Commands: []*cli.Command{
			commands.Config,
			commands.Implode,
			commands.Hooks,
			commands.Add,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
