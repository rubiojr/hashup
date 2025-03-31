package main

import (
	"fmt"
	"runtime/debug"

	"github.com/urfave/cli/v2"
)

func commandVersion() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "HashUp version",
		Flags: []cli.Flag{},
		Action: func(c *cli.Context) error {
			if bi, ok := debug.ReadBuildInfo(); ok {
				fmt.Println(bi.Main.Version)
			}
			return nil
		},
	}
}
