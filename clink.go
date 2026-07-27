package main

import (
	"context"
	"fmt"

	"fiatjaf.com/nostr"
	"github.com/fatih/color"
	"github.com/shocknet/clink-go/clink"
	"github.com/urfave/cli/v3"
)

const (
	bootstrapPubHex = "76ed45f00cea7bac59d8d0b7d204848f5319d7b96c140ffb6fcbaaab0a13d44e"
	bootstrapRelay  = "wss://relay.lightning.pub"
)

var clinkFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "pub",
		Usage: "Lightning.Pub app pubkey (hex)",
		Value: bootstrapPubHex,
	},
	&cli.StringFlag{
		Name:    "relay",
		Aliases: []string{"r"},
		Usage:   "relay used for Pub RPC (kind 21000)",
		Value:   bootstrapRelay,
	},
}

var clinkInfo = &cli.Command{
	Name:  "info",
	Usage: "GetUserInfo from a Lightning.Pub (default offer, balance)",
	DisableSliceFlagSeparator: true,
	Flags:                     clinkFlags,
	Action: func(ctx context.Context, c *cli.Command) error {
		pub, err := pubFromCLI(ctx, c)
		if err != nil {
			return err
		}
		res, err := pub.GetUserInfo(ctx)
		if err != nil {
			return err
		}
		stdout("balance:", clink.AsInt64(res["balance"]))
		stdout("max_withdrawable:", clink.AsInt64(res["max_withdrawable"]))
		stdout("user_identifier:", res["user_identifier"])
		stdout("noffer:", res["noffer"])
		stdout("ndebit:", res["ndebit"])
		stdout("nmanage:", res["nmanage"])
		return nil
	},
}

var clinkOffer = &cli.Command{
	Name:                      "offer",
	Usage:                     "print default noffer, or create one with --label via AddUserOffer",
	DisableSliceFlagSeparator: true,
	Flags: append(clinkFlags,
		&cli.StringFlag{Name: "label", Usage: "if set, create a new user offer with this label"},
		&cli.IntFlag{Name: "sats", Usage: "price_sats for new offer (0 = spontaneous)", Value: 0},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		pub, err := pubFromCLI(ctx, c)
		if err != nil {
			return err
		}
		label := c.String("label")
		if label == "" {
			res, err := pub.GetUserInfo(ctx)
			if err != nil {
				return err
			}
			stdout(res["noffer"])
			return nil
		}

		log("creating offer %s...\n", color.CyanString(label))
		created, err := pub.AddUserOffer(ctx, map[string]any{
			"label":              label,
			"price_sats":         c.Int("sats"),
			"callback_url":       "",
			"payer_data":         []string{},
			"token":              "",
			"rejectUnauthorized": false,
		})
		if err != nil {
			return err
		}
		offerID, _ := created["offer_id"].(string)
		got, err := pub.GetUserOffer(ctx, offerID)
		if err != nil {
			return err
		}
		stdout(got["noffer"])
		return nil
	},
}

var clinkInvoice = &cli.Command{
	Name:                      "invoice",
	Usage:                     "request a BOLT11 invoice from a noffer1… (kind 21001)",
	ArgsUsage:                 "<noffer1...>",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.IntFlag{Name: "amount", Aliases: []string{"a"}, Usage: "amount_sats (required for spontaneous)", Value: 0},
		&cli.StringFlag{Name: "description", Aliases: []string{"d"}, Usage: "optional memo (<=100 chars)"},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		offer, err := clink.DecodeNoffer(c.Args().First())
		if err != nil {
			return err
		}
		client, err := clientEphemeralOrSec(ctx, c)
		if err != nil {
			return err
		}
		bolt11, err := client.Noffer(ctx, offer, int64(c.Int("amount")), c.String("description"))
		if err != nil {
			return err
		}
		stdout(bolt11)
		return nil
	},
}

var clinkPay = &cli.Command{
	Name:                      "pay",
	Usage:                     "request invoice from noffer then PayInvoice via Lightning.Pub",
	ArgsUsage:                 "<noffer1...>",
	DisableSliceFlagSeparator: true,
	Flags: append(clinkFlags,
		&cli.IntFlag{Name: "amount", Aliases: []string{"a"}, Usage: "amount_sats for spontaneous offers", Value: 0},
		&cli.StringFlag{Name: "description", Aliases: []string{"d"}, Usage: "optional memo"},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		offer, err := clink.DecodeNoffer(c.Args().First())
		if err != nil {
			return err
		}
		client, err := clientFromSec(ctx, c)
		if err != nil {
			return err
		}

		log("requesting invoice from noffer...\n")
		bolt11, err := client.Noffer(ctx, offer, int64(c.Int("amount")), c.String("description"))
		if err != nil {
			return err
		}
		log("bolt11: %s\n", color.YellowString(bolt11))

		pub, err := pubFromClient(c, client)
		if err != nil {
			return err
		}
		log("PayInvoice via Pub RPC...\n")
		res, err := pub.PayInvoice(ctx, bolt11)
		if err != nil {
			return fmt.Errorf("PayInvoice: %w", err)
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		stdout(string(b))
		return nil
	},
}

var clinkDecode = &cli.Command{
	Name:                      "decode-noffer",
	Usage:                     "decode a noffer1… TLV locally (no network)",
	ArgsUsage:                 "<noffer1...>",
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		for input := range getStdinLinesOrArguments(c.Args()) {
			offer, err := clink.DecodeNoffer(input)
			if err != nil {
				return err
			}
			stdout("pubkey:", offer.Pubkey.Hex())
			stdout("relay:", offer.Relay)
			stdout("offer:", offer.Offer)
			stdout("priceType:", clink.PriceTypeName(offer.PriceType))
			if offer.HasPrice {
				stdout("price:", offer.Price)
			}
		}
		return nil
	},
}

func clientFromSec(ctx context.Context, c *cli.Command) (*clink.Client, error) {
	_, sk, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return nil, err
	}
	return clink.NewClient(sys.Pool, sk), nil
}

func clientEphemeralOrSec(ctx context.Context, c *cli.Command) (*clink.Client, error) {
	if c.IsSet("sec") || c.Bool("prompt-sec") {
		return clientFromSec(ctx, c)
	}
	sk, err := clink.EphemeralKey()
	if err != nil {
		return nil, err
	}
	log("using ephemeral payer key %s\n", color.CyanString(sk.Public().Hex()))
	return clink.NewClient(sys.Pool, sk), nil
}

func pubFromCLI(ctx context.Context, c *cli.Command) (*clink.Pub, error) {
	client, err := clientFromSec(ctx, c)
	if err != nil {
		return nil, err
	}
	return pubFromClient(c, client)
}

func pubFromClient(c *cli.Command, client *clink.Client) (*clink.Pub, error) {
	dest, err := nostr.PubKeyFromHex(c.String("pub"))
	if err != nil {
		return nil, fmt.Errorf("invalid --pub: %w", err)
	}
	relay := c.String("relay")
	if relay == "" {
		return nil, fmt.Errorf("missing --relay")
	}
	return client.Pub(dest, []string{relay}), nil
}
