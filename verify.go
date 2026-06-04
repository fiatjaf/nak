package main

import (
	"context"

	"fiatjaf.com/nostr"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var verify = &cli.Command{
	Name:  "verify",
	Usage: "checks the hash and signature of an event given through stdin or as the first argument",
	Description: `example:
		echo '{"id":"a889df6a387419ff204305f4c2d296ee328c3cd4f8b62f205648a541b4554dfb","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698623783,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"84876e1ee3e726da84e5d195eb79358b2b3eaa4d9bd38456fde3e8a2af3f1cd4cda23f23fda454869975b3688797d4c66e12f4c51c1b43c6d2997c5e61865661"}' | nak verify

it outputs nothing if the verification is successful.`,
	DisableSliceFlagSeparator: true,
	Action: func(ctx context.Context, c *cli.Command) error {
		for stdinEvent := range getJsonsOrBlank() {
			evt := nostr.Event{}
			if stdinEvent == "" {
				stdinEvent = c.Args().First()
				if stdinEvent == "" {
					continue
				}
			}

			if err := json.Unmarshal([]byte(stdinEvent), &evt); err != nil {
				ctx = lineProcessingError(ctx, "invalid event: %s", err)
				logverbose("%s\n", color.RedString("<>: invalid event."))
				continue
			}

			impliedID := evt.GetID()
			idsMatch := impliedID == evt.ID
			logverbose(
				"%s\n%s %s\n%s %s\n%s %s\n%s %s\n%s %s\n",
				color.CyanString("verifying event:"),
				color.BlueString("  event:     "), stdinEvent,
				color.BlueString("  given id:  "), color.YellowString("%s", evt.ID),
				color.BlueString("  serialized:"), string(evt.Serialize()),
				color.BlueString("  implied id:"), color.YellowString("%s", impliedID),
				color.BlueString("  ids match: "), color.New(map[bool]color.Attribute{true: color.FgGreen, false: color.FgRed}[idsMatch]).Sprint(idsMatch),
			)

			if impliedID != evt.ID {
				ctx = lineProcessingError(ctx, "invalid .id, expected %s, got %s", impliedID, evt.ID)
				logverbose("%s\n", color.RedString("invalid id: %s", evt.ID.Hex()))
				continue
			}

			if !evt.VerifySignature() {
				ctx = lineProcessingError(ctx, "invalid signature")
				logverbose("%s\n", color.RedString("invalid signature: %s", evt.ID.Hex()))
				continue
			}

			logverbose("%s\n", color.GreenString("valid: %s", evt.ID.Hex()))
		}

		exitIfLineProcessingError(ctx)
		return nil
	},
}
