package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

var app = &cli.App{
	Name:  "nak",
	Usage: "the nostr army knife command-line tool",
	Commands: []*cli.Command{
		req,
		count,
		fetch,
		event,
		decode,
		encode,
		verify,
		relay,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "silent",
			Usage:   "do not print logs and info messages to stderr",
			Aliases: []string{"s"},
			Action: func(ctx *cli.Context, b bool) error {
				if b {
					log = func(msg string, args ...any) {}
				}
				return nil
			},
		},
	},
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
