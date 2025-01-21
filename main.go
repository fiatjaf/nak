package main

import (
	"context"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"

	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/sdk"
	"github.com/nbd-wtf/go-nostr/sdk/hints/memoryh"
	"github.com/fatih/color"
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
		outbox,
	},
	Version: version,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:       "config-path",
			Hidden:     true,
			Persistent: true,
		},
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
	Before: func(ctx context.Context, c *cli.Command) error {
		configPath := c.String("config-path")
		if configPath == "" {
			if home, err := os.UserHomeDir(); err == nil {
				configPath = filepath.Join(home, ".config/nak")
			}
		}
		if configPath != "" {
			hintsFilePath = filepath.Join(configPath, "outbox/hints.db")
		}
		if hintsFilePath != "" {
			if _, err := os.Stat(hintsFilePath); !os.IsNotExist(err) {
				hintsFileExists = true
			}
		}

		if hintsFilePath != "" {
			if data, err := os.ReadFile(hintsFilePath); err == nil {
				hintsdb := memoryh.NewHintDB()
				if err := json.Unmarshal(data, &hintsdb); err == nil {
					sys = sdk.NewSystem(
						sdk.WithHintsDB(hintsdb),
					)
					goto systemOperational
				}
			}
		}

		sys = sdk.NewSystem()

	systemOperational:
		sys.Pool = nostr.NewSimplePool(context.Background(),
			nostr.WithAuthorKindQueryMiddleware(sys.TrackQueryAttempts),
			nostr.WithEventMiddleware(sys.TrackEventHints),
			nostr.WithRelayOptions(
				nostr.WithRequestHeader(http.Header{textproto.CanonicalMIMEHeaderKey("user-agent"): {"nak/b"}}),
			),
		)

		return nil
	},
	After: func(ctx context.Context, c *cli.Command) error {
		// save hints database on exit
		if hintsFileExists {
			data, err := json.Marshal(sys.Hints)
			if err != nil {
				return err
			}
			return os.WriteFile(hintsFilePath, data, 0644)
		}

		return nil
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
