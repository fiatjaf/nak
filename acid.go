package main

import (
	"context"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/urfave/cli/v2"
)

var acid = &cli.Command{
	Name:  "acid",
	Usage: "tests a relay for spec compliance",
	Description: `example:
		nak acid nostr.wine`,
	ArgsUsage: "<relay-url>",
	Action: func(c *cli.Context) error {
		url := c.Args().Get(0)

		if url == "" {
			return fmt.Errorf("specify the <relay-url>")
		}

		ctx := context.Background()
		relay, err := nostr.RelayConnect(ctx, url)
		if err != nil {
			panic(err)
		}

		sk := nostr.GeneratePrivateKey()
		pub, _ := nostr.GetPublicKey(sk)

		// Test 1: Get the relay's information document
		_, err11 := nip11.Fetch(c.Context, url)
		if err11 != nil {
			fmt.Println("NIP-11... FAIL")
		} else {
			fmt.Println("NIP-11... PASS")
		}

		// Test 2: Publish an event
		ev := nostr.Event{
			PubKey:    pub,
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindTextNote,
			Tags:      nil,
			Content:   "Hello World!",
		}
		ev.Sign(sk)

		if publishErr := relay.Publish(ctx, ev); publishErr != nil {
			fmt.Println("Publish event... FAIL")
		} else {
			fmt.Println("Publish event... PASS")
		}

		return nil
	},
}
