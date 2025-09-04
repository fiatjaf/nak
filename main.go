package main

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/sdk"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var (
	version   string = "debug"
	isVerbose bool   = false
)

var app = &cli.Command{
	Name:                      "nak",
	Suggest:                   true,
	UseShortOptionHandling:    true,
	Usage:                     "the nostr army knife command-line tool",
	DisableSliceFlagSeparator: true,
	Commands: []*cli.Command{
		event,
		req,
		filter,
		fetch,
		count,
		decode,
		encode,
		key,
		verify,
		relay,
		admin,
		bunker,
		serve,
		blossomCmd,
		encrypt,
		decrypt,
		outbox,
		wallet,
		mcpServer,
		curl,
		fsCmd,
		publish,
	},
	Version: version,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:   "config-path",
			Hidden: true,
			Value: (func() string {
				if home, err := os.UserHomeDir(); err == nil {
					return filepath.Join(home, ".config/nak")
				} else {
					return filepath.Join("/dev/null")
				}
			})(),
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Usage:   "do not print logs and info messages to stderr, use -qq to also not print anything to stdout",
			Aliases: []string{"q"},
			Action: func(ctx context.Context, c *cli.Command, b bool) error {
				q := c.Count("quiet")
				if q >= 1 {
					log = func(msg string, args ...any) {}
					if q >= 2 {
						stdout = func(_ ...any) {}
					}
				}
				return nil
			},
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Usage:   "print more stuff than normally",
			Aliases: []string{"v"},
			Action: func(ctx context.Context, c *cli.Command, b bool) error {
				v := c.Count("verbose")
				if v >= 1 {
					logverbose = log
					isVerbose = true
				}
				return nil
			},
		},
	},
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		sys = sdk.NewSystem()

		if err := initializeOutboxHintsDB(c, sys); err != nil {
			return ctx, fmt.Errorf("failed to initialize outbox hints: %w", err)
		}

		sys.Pool = nostr.NewPool(nostr.PoolOptions{
			AuthorKindQueryMiddleware: sys.TrackQueryAttempts,
			EventMiddleware:           sys.TrackEventHints,
			RelayOptions: nostr.RelayOptions{
				RequestHeader: http.Header{textproto.CanonicalMIMEHeaderKey("user-agent"): {"nak/b"}},
			},
		})

		return ctx, nil
	},
}

func init() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:  "version",
		Usage: "prints the version",
	}
}

func main() {
	defer colors.reset()

	// a megahack to enable this curl command proxy
	if len(os.Args) > 2 && os.Args[1] == "curl" {
		if err := realCurl(); err != nil {
			if err != nil {
				log(color.YellowString(err.Error()) + "\n")
			}
			colors.reset()
			os.Exit(1)
		}
		return
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		if err != nil {
			log("%s\n", color.RedString(err.Error()))
		}
		colors.reset()
		os.Exit(1)
	}
}
