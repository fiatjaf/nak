package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip86"
	"github.com/urfave/cli/v3"
)

var admin = &cli.Command{
	Name:  "admin",
	Usage: "manage relays using the relay management API",
	Description: `examples:
		nak admin allowpubkey myrelay.com --pubkey 1234... --reason "good user"
		nak admin banpubkey myrelay.com --pubkey 1234... --reason "spam"
		nak admin listallowedpubkeys myrelay.com
		nak admin changerelayname myrelay.com --name "My Relay"`,
	ArgsUsage:                 "<relay-url>",
	DisableSliceFlagSeparator: true,
	Flags:                     defaultKeyFlags,
	Commands: (func() []*cli.Command {
		methods := []struct {
			method string
			args   []string
		}{
			{"allowpubkey", []string{"pubkey", "reason"}},
			{"banpubkey", []string{"pubkey", "reason"}},
			{"listallowedpubkeys", nil},
			{"listbannedpubkeys", nil},
			{"listeventsneedingmoderation", nil},
			{"allowevent", []string{"id", "reason"}},
			{"banevent", []string{"id", "reason"}},
			{"listbannedevents", nil},
			{"changerelayname", []string{"name"}},
			{"changerelaydescription", []string{"description"}},
			{"changerelayicon", []string{"icon"}},
			{"allowkind", []string{"kind"}},
			{"disallowkind", []string{"kind"}},
			{"listallowedkinds", nil},
			{"blockip", []string{"ip", "reason"}},
			{"unblockip", []string{"ip", "reason"}},
			{"listblockedips", nil},
		}

		commands := make([]*cli.Command, 0, len(methods))
		for _, def := range methods {
			def := def

			flags := make([]cli.Flag, len(def.args), len(def.args)+4)
			for i, argName := range def.args {
				flags[i] = declareFlag(argName)
			}

			cmd := &cli.Command{
				Name:  def.method,
				Usage: fmt.Sprintf(`the "%s" relay management RPC call`, def.method),
				Description: fmt.Sprintf(
					`the "%s" management RPC call, see https://nips.nostr.com/86 for more information`, def.method),
				Flags:                     flags,
				DisableSliceFlagSeparator: true,
				Action: func(ctx context.Context, c *cli.Command) error {
					params := make([]any, len(def.args))
					for i, argName := range def.args {
						params[i] = getArgument(c, argName)
					}
					req := nip86.Request{Method: def.method, Params: params}
					reqj, _ := json.Marshal(req)

					relayUrls := c.Args().Slice()
					if len(relayUrls) == 0 {
						stdout(string(reqj))
						return nil
					}

					kr, _, err := gatherKeyerFromArguments(ctx, c)
					if err != nil {
						return err
					}

					for _, relayUrl := range relayUrls {
						httpUrl := "http" + nostr.NormalizeURL(relayUrl)[2:]
						log("calling '%s' on %s... ", def.method, httpUrl)
						body := bytes.NewBuffer(nil)
						body.Write(reqj)
						req, err := http.NewRequestWithContext(ctx, "POST", httpUrl, body)
						if err != nil {
							return fmt.Errorf("failed to create request: %w", err)
						}

						// Authorization
						payloadHash := sha256.Sum256(reqj)
						tokenEvent := nostr.Event{
							Kind:      27235,
							CreatedAt: nostr.Now(),
							Tags: nostr.Tags{
								{"u", httpUrl},
								{"method", "POST"},
								{"payload", hex.EncodeToString(payloadHash[:])},
							},
						}
						if err := kr.SignEvent(ctx, &tokenEvent); err != nil {
							return fmt.Errorf("failed to sign token event: %w", err)
						}
						evtj, _ := json.Marshal(tokenEvent)
						req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(evtj))

						// Content-Type
						req.Header.Set("Content-Type", "application/nostr+json+rpc")

						// make request to relay
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							log("failed: %s\n", err)
							continue
						}
						b, err := io.ReadAll(resp.Body)
						if err != nil {
							log("failed to read response: %s\n", err)
							continue
						}
						if resp.StatusCode >= 300 {
							log("failed with status %d\n", resp.StatusCode)
							bodyPrintable := string(b)
							if len(bodyPrintable) > 300 {
								bodyPrintable = bodyPrintable[0:297] + "..."
							}
							log(bodyPrintable)
							continue
						}
						var response nip86.Response
						if err := json.Unmarshal(b, &response); err != nil {
							log("bad json response: %s\n", err)
							bodyPrintable := string(b)
							if len(bodyPrintable) > 300 {
								bodyPrintable = bodyPrintable[0:297] + "..."
							}
							log(bodyPrintable)
							continue
						}
						resp.Body.Close()

						// print the result
						log("\n")
						pretty, _ := json.MarshalIndent(response, "", "  ")
						stdout(string(pretty))
					}

					return nil
				},
			}

			commands = append(commands, cmd)
		}

		return commands
	})(),
}

func declareFlag(argName string) cli.Flag {
	usage := "parameter for this management RPC call, see https://nips.nostr.com/86 for more information."
	switch argName {
	case "kind":
		return &cli.IntFlag{Name: argName, Required: true, Usage: usage}
	case "reason":
		return &cli.StringFlag{Name: argName, Usage: usage}
	default:
		return &cli.StringFlag{Name: argName, Required: true, Usage: usage}
	}
}

func getArgument(c *cli.Command, argName string) any {
	switch argName {
	case "kind":
		return c.Int(argName)
	default:
		return c.String(argName)
	}
}
