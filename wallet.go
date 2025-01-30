package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip60"
)

func prepareWallet(ctx context.Context, c *cli.Command) (*nip60.WalletStash, func(), error) {
	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return nil, nil, err
	}

	pk, err := kr.GetPublicKey(ctx)
	if err != nil {
		return nil, nil, err
	}

	relays := sys.FetchOutboxRelays(ctx, pk, 3)
	wl := nip60.LoadStash(ctx, kr, sys.Pool, relays)
	if wl == nil {
		return nil, nil, fmt.Errorf("error loading wallet stash")
	}

	wl.Processed = func(evt *nostr.Event, err error) {
		if err == nil {
			logverbose("processed event %s\n", evt)
		} else {
			log("error processing event %s: %s\n", evt, err)
		}
	}

	wl.PublishUpdate = func(event nostr.Event, deleted, received, change *nip60.Token, isHistory bool) {
		desc := "wallet"
		if received != nil {
			desc = fmt.Sprintf("received from %s with %d proofs totalling %d",
				received.Mint, len(received.Proofs), received.Proofs.Amount())
		} else if change != nil {
			desc = fmt.Sprintf("change from %s with %d proofs totalling %d",
				change.Mint, len(change.Proofs), change.Proofs.Amount())
		} else if deleted != nil {
			desc = fmt.Sprintf("deleting a used token from %s with %d proofs totalling %d",
				deleted.Mint, len(deleted.Proofs), deleted.Proofs.Amount())
		} else if isHistory {
			desc = "history entry"
		}

		log("- saving kind:%d event (%s)... ", event.Kind, desc)
		first := true
		for res := range sys.Pool.PublishMany(ctx, relays, event) {
			cleanUrl, ok := strings.CutPrefix(res.RelayURL, "wss://")
			if !ok {
				cleanUrl = res.RelayURL
			}

			if !first {
				log(", ")
			}
			first = false

			if res.Error != nil {
				log("%s: %s", color.RedString(cleanUrl), res.Error)
			} else {
				log("%s: ok", color.GreenString(cleanUrl))
			}
		}
		log("\n")
	}

	<-wl.Stable

	return wl, func() {
		wl.Close()
	}, nil
}

var wallet = &cli.Command{
	Name:                      "wallet",
	Usage:                     "manage nip60 cashu wallets (see subcommands)",
	Description:               "all wallet data is stored on Nostr relays, signed and encrypted with the given key, and reloaded again from relays on every call.\n\nthe same data can be accessed by other compatible nip60 clients.",
	DisableSliceFlagSeparator: true,
	Flags:                     defaultKeyFlags,
	ArgsUsage:                 "<wallet-identifier>",
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args().Slice()
		if len(args) != 1 {
			return fmt.Errorf("must be called as `nak wallet <wallet-id>")
		}

		wl, closewl, err := prepareWallet(ctx, c)
		if err != nil {
			return err
		}

		for w := range wl.Wallets() {
			if w.Identifier == args[0] {
				stdout(w.DisplayName(), w.Balance())
				break
			}
		}

		closewl()
		return nil
	},
	Commands: []*cli.Command{
		{
			Name:                      "list",
			Usage:                     "lists existing wallets with their balances",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				wl, closewl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				for w := range wl.Wallets() {
					stdout(w.DisplayName(), w.Balance())
				}

				closewl()
				return nil
			},
		},
		{
			Name:                      "tokens",
			Usage:                     "lists existing tokens in this wallet with their mints and aggregated amounts",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				wl, closewl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				args := c.Args().Slice()
				if len(args) != 1 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> tokens")
				}

				w := wl.EnsureWallet(ctx, args[0])

				for _, token := range w.Tokens {
					stdout(token.ID(), token.Proofs.Amount(), strings.Split(token.Mint, "://")[1])
				}

				closewl()
				return nil
			},
		},
		{
			Name:                      "receive",
			Usage:                     "takes a cashu token string as an argument and adds it to the wallet",
			ArgsUsage:                 "<token>",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args().Slice()
				if len(args) != 2 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> receive <token>")
				}

				wl, closewl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				w := wl.EnsureWallet(ctx, args[0])

				if err := w.ReceiveToken(ctx, args[1]); err != nil {
					return err
				}

				closewl()
				return nil
			},
		},
		{
			Name:                      "send",
			Usage:                     "prints a cashu token with the given amount for sending to someone else",
			ArgsUsage:                 "<amount>",
			DisableSliceFlagSeparator: true,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "mint",
					Usage: "send from a specific mint",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args().Slice()
				if len(args) != 2 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> send <amount>")
				}
				amount, err := strconv.ParseUint(args[1], 10, 64)
				if err != nil {
					return fmt.Errorf("amount '%s' is invalid", args[1])
				}

				wl, closewl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				w := wl.EnsureWallet(ctx, args[0])

				opts := make([]nip60.SendOption, 0, 1)
				if mint := c.String("mint"); mint != "" {
					mint = "http" + nostr.NormalizeURL(mint)[2:]
					opts = append(opts, nip60.WithMint(mint))
				}
				token, err := w.SendToken(ctx, amount, opts...)
				if err != nil {
					return err
				}

				stdout(token)

				closewl()
				return nil
			},
		},
		{
			Name:                      "pay",
			Usage:                     "pays a bolt11 lightning invoice and outputs the preimage",
			ArgsUsage:                 "<invoice>",
			DisableSliceFlagSeparator: true,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "mint",
					Usage: "pay from a specific mint",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args().Slice()
				if len(args) != 2 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> pay <invoice>")
				}

				wl, closewl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				w := wl.EnsureWallet(ctx, args[0])

				opts := make([]nip60.SendOption, 0, 1)
				if mint := c.String("mint"); mint != "" {
					mint = "http" + nostr.NormalizeURL(mint)[2:]
					opts = append(opts, nip60.WithMint(mint))
				}

				preimage, err := w.PayBolt11(ctx, args[1], opts...)
				if err != nil {
					return err
				}

				stdout(preimage)

				closewl()
				return nil
			},
		},
	},
}
