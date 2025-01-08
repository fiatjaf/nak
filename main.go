package main

import (
	"context"
	"os"

	"github.com/fatih/color"
	"github.com/fiatjaf/cli/v3"
)

var version string = "debug"

var app = &cli.Command{
	Name:                      "nak",
	Suggest:                   true,
	UseShortOptionHandling:    true,
	AllowFlagsAfterArguments:  true,
	Usage:                     "the nostr army knife command-line tool",
	DisableSliceFlagSeparator: true,
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
		serve,
		encrypt,
		decrypt,
	},
	Version: version,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:       "quiet",
			Usage:      "do not print logs and info messages to stderr, use -qq to also not print anything to stdout",
			Aliases:    []string{"q"},
			Persistent: true,
			Action: func(ctx context.Context, c *cli.Command, b bool) error {
				q := c.Count("quiet")
				if q >= 1 {
					log = func(msg string, args ...any) {}
					if q >= 2 {
						stdout = func(_ ...any) (int, error) { return 0, nil }
					}
				}
				return nil
			},
		},
	},
}

func main() {
	defer func() {
		color.New(color.Reset).Println()
	}()
	if err := app.Run(context.Background(), os.Args); err != nil {
		stdout(err)
		color.New(color.Reset).Println()
		os.Exit(1)
	}
}
