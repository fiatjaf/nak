package main

import (
	"encoding/json"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli/v2"
)

var verify = &cli.Command{
	Name:  "verify",
	Usage: "checks the hash and signature of an event given through stdin",
	Description: `example:
		echo '{"id":"a889df6a387419ff204305f4c2d296ee328c3cd4f8b62f205648a541b4554dfb","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698623783,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"84876e1ee3e726da84e5d195eb79358b2b3eaa4d9bd38456fde3e8a2af3f1cd4cda23f23fda454869975b3688797d4c66e12f4c51c1b43c6d2997c5e61865661"}' | nak verify

it outputs nothing if the verification is successful.
`,
	Action: func(c *cli.Context) error {
		evt := nostr.Event{}
		if stdinEvent := getStdin(); stdinEvent != "" {
			if err := json.Unmarshal([]byte(stdinEvent), &evt); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
		}

		if evt.GetID() != evt.ID {
			return fmt.Errorf("invalid .id, expected %s, got %s", evt.GetID(), evt.ID)
		}

		if ok, err := evt.CheckSignature(); !ok {
			return fmt.Errorf("invalid signature: %w", err)
		}

		return nil
	},
}
