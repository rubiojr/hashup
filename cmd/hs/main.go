package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "hs",
		Usage: "Search for files in a Hashub database",
	}

	app.Commands = append(
		app.Commands,
		commandSearch(),
		commandHosts(),
		commandFileStats(),
		commandLargeFiles(),
		commandTag(),
		commandTags(),
		commandAdmin(),
		commandVersion(),
	)

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
