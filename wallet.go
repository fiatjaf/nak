package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fiatjaf/cli/v3"
	"github.com/nbd-wtf/go-nostr/nip60"
)

func prepareWallet(ctx context.Context, c *cli.Command) (*nip60.WalletStash, error) {
	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return nil, err
	}

	pk, err := kr.GetPublicKey(ctx)
	if err != nil {
		return nil, err
	}

	relays := sys.FetchOutboxRelays(ctx, pk, 3)
	wl := nip60.LoadStash(ctx, kr, sys.Pool, relays)
	if wl == nil {
		return nil, fmt.Errorf("error loading wallet stash")
	}

	go func() {
		for err := range wl.Processed {
			if err == nil {
				// event processed ok
			} else {
				log("processing error: %s\n", err)
			}
		}
	}()

	go func() {
		for evt := range wl.Changes {
			for res := range sys.Pool.PublishMany(ctx, relays, evt) {
				if res.Error != nil {
					log("error saving kind:%d event to %s: %s\n", evt.Kind, res.RelayURL, err)
				} else {
					log("saved kind:%d event to %s\n", evt.Kind, res.RelayURL)
				}
			}
		}
	}()

	<-wl.Stable

	return wl, nil
}

var wallet = &cli.Command{
	Name:                      "wallet",
	Usage:                     "manage nip60 Cashu wallets",
	DisableSliceFlagSeparator: true,
	Flags:                     defaultKeyFlags,
	ArgsUsage:                 "<wallet-identifier>",
	Commands: []*cli.Command{
		{
			Name:                      "list",
			Usage:                     "list existing Cashu wallets",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				wl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				for w := range wl.Wallets() {
					stdout(w.DisplayName(), w.Balance())
				}

				return nil
			},
		},
		{
			Name:                      "tokens",
			Usage:                     "list existing tokens in this wallet",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				wl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				args := c.Args().Slice()
				if len(args) != 1 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> tokens")
				}

				w := wl.EnsureWallet(args[0])

				for _, token := range w.Tokens {
					stdout(token.ID(), token.Proofs.Amount(), token.Mint)
				}

				return nil
			},
		},
		{
			Name:                      "receive",
			Usage:                     "receive a cashu token",
			ArgsUsage:                 "<token>",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args().Slice()
				if len(args) != 2 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> receive <token>")
				}

				wl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				w := wl.EnsureWallet(args[0])

				if err := w.ReceiveToken(ctx, args[1]); err != nil {
					return err
				}

				return nil
			},
		},
		{
			Name:                      "send",
			Usage:                     "send a cashu token",
			ArgsUsage:                 "<amount>",
			DisableSliceFlagSeparator: true,
			Action: func(ctx context.Context, c *cli.Command) error {
				args := c.Args().Slice()
				if len(args) != 2 {
					return fmt.Errorf("must be called as `nak wallet <wallet-id> send <amount>")
				}
				amount, err := strconv.ParseUint(args[1], 10, 64)
				if err != nil {
					return fmt.Errorf("amount '%s' is invalid", args[1])
				}

				wl, err := prepareWallet(ctx, c)
				if err != nil {
					return err
				}

				w := wl.EnsureWallet(args[0])

				token, err := w.SendToken(ctx, amount)
				if err != nil {
					return err
				}

				stdout(token)

				return nil
			},
		},
	},
}
