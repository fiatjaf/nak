package main

import (
	"context"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/sdk"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var authFlags = []cli.Flag{
	&cli.BoolFlag{
		Name:     "auth",
		Usage:    "always perform nip42 \"AUTH\" when facing an \"auth-required: \" rejection and try again",
		Category: CATEGORY_AUTH,
	},
	&cli.BoolFlag{
		Name:     "force-pre-auth",
		Aliases:  []string{"fpa"},
		Usage:    "after connecting, wait for a nip42 \"AUTH\" message to be received, act on it and only then send the query",
		Category: CATEGORY_AUTH,
	},
}

var defaultKeyFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "sec",
		Usage:       "secret key to sign the event, as nsec, ncryptsec or hex, or a bunker URL",
		Category:    CATEGORY_SIGNER,
		Sources:     cli.EnvVars("NOSTR_SECRET_KEY"),
		Value:       defaultKey().Hex(),
		DefaultText: "a default key specific to your machine, see it with `nak key default`",
	},
	&cli.BoolFlag{
		Name:     "prompt-sec",
		Usage:    "prompt the user to paste a hex or nsec with which to sign the event",
		Category: CATEGORY_SIGNER,
		Hidden:   true,
	},
	&SecretKeyFlag{
		Name:        "connect-as",
		Usage:       "private key to use when communicating with nip46 bunkers",
		Category:    CATEGORY_SIGNER,
		Sources:     cli.EnvVars("NOSTR_CLIENT_KEY"),
		Value:       defaultKey(),
		DefaultText: "the default key (see `nak key default`)",
		Hidden:      true,
	},
}

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
		eventCmd,
		req,
		filterCmd,
		fetch,
		count,
		decode,
		encode,
		key,
		kindCmd,
		verify,
		relay,
		admin,
		bunker,
		serve,
		blossomCmd,
		dekey,
		encrypt,
		decrypt,
		gift,
		outboxCmd,
		mcpServer,
		curl,
		fsCmd,
		publish,
		nsite,
		git,
		group,
		podcast,
		nip,
		syncCmd,
		kanban,
		spell,
		profile,
		validateCmd,
	},
	Version: version,
	Flags: combineFlags([][]cli.Flag{
		{
			&cli.StringFlag{
				Name:   "config-path",
				Hidden: true,
				Value: (func() string {
					if home, err := os.UserHomeDir(); err == nil {
						return filepath.Join(home, ".config/nak")
					} else {
						return ""
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
				Hidden: true,
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
				Hidden: true,
			},
			&cli.DurationFlag{
				Name:  "connect-timeout",
				Usage: "timeout for connecting to relays",
				Value: connectTimeout,
				Action: func(ctx context.Context, c *cli.Command, d time.Duration) error {
					connectTimeout = d
					return nil
				},
				Hidden: true,
			},
		},
		defaultKeyFlags,
		authFlags,
	}),
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		sys = sdk.NewSystem()

		setupLocalDatabases(c, sys)

		sys.Pool.QueryMiddleware = sys.TrackQueryAttempts
		sys.Pool.EventMiddleware = sys.TrackEventHints
		sys.Pool.RelayOptions = nostr.RelayOptions{
			RequestHeader: http.Header{textproto.CanonicalMIMEHeaderKey("user-agent"): {"nak/b"}},
		}
		sys.Pool.AuthRequiredHandler = func(ctx context.Context, authEvent *nostr.Event) error {
			return authSigner(ctx, c, func(s string, args ...any) {
				if strings.HasPrefix(s, "authenticating as") {
					cleanUrl, _ := strings.CutPrefix(
						nip42.GetRelayURLFromAuthEvent(*authEvent),
						"wss://",
					)
					s = "authenticating to " + color.CyanString(cleanUrl) + " as" + s[len("authenticating as"):]
				}
				log(s+"\n", args...)
			}, authEvent)
		}

		return ctx, nil
	},
}

func init() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:  "version",
		Usage: "prints version",
	}
	cli.HelpFlag = &cli.BoolFlag{
		Name:        "help",
		Usage:       "shows help",
		HideDefault: true,
		Local:       true,
	}
}

func main() {
	defer colors.reset()

	// a megahack to enable this curl command proxy
	if len(os.Args) > 2 && os.Args[1] == "curl" {
		if err := realCurl(); err != nil {
			log(color.YellowString(err.Error()) + "\n")
			colors.reset()
			os.Exit(1)
		}
		return
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log("%s\n", color.RedString(err.Error()))
		colors.reset()
		os.Exit(1)
	}
}
