package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"
)

var app = &cli.Command{
	Name:                     "nak",
	Suggest:                  true,
	UseShortOptionHandling:   true,
	AllowFlagsAfterArguments: true,
	Usage:                    "the nostr army knife command-line tool",
	Commands: []*cli.Command{
		req,
		count,
		fetch,
		event,
		decode,
		encode,
		key,
		verify,
		relay,
		bunker,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Usage:   "do not print logs and info messages to stderr, use -qq to also not print anything to stdout",
			Aliases: []string{"q"},
			Action: func(ctx context.Context, c *cli.Command, b bool) error {
				q := c.Count("quiet")
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
	if err := app.Run(context.Background(), os.Args); err != nil {
		stdout(err)
		os.Exit(1)
	}
}
