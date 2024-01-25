package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

var q int

var app = &cli.App{
	Name:                   "nak",
	Suggest:                true,
	UseShortOptionHandling: true,
	Usage:                  "the nostr army knife command-line tool",
	Commands: []*cli.Command{
		req,
		count,
		fetch,
		event,
		decode,
		encode,
		verify,
		relay,
		bunker,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Usage:   "do not print logs and info messages to stderr, use -qq to also not print anything to stdout",
			Count:   &q,
			Aliases: []string{"q"},
			Action: func(ctx *cli.Context, b bool) error {
				if q >= 1 {
					log = func(msg string, args ...any) {}
					if q >= 2 {
						stdout = func(a ...any) (int, error) { return 0, nil }
					}
				}
				return nil
			},
		},
	},
}

func main() {
	if err := app.Run(os.Args); err != nil {
		stdout(err)
		os.Exit(1)
	}
}
