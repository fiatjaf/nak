package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fiatjaf/cli/v3"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nbd-wtf/go-nostr"
)

var mcpServer = &cli.Command{
	Name:                      "mcp",
	Usage:                     "pander to the AI gods",
	Description:               ``,
	DisableSliceFlagSeparator: true,
	Flags:                     []cli.Flag{},
	Action: func(ctx context.Context, c *cli.Command) error {
		s := server.NewMCPServer(
			"nak",
			version,
		)

		s.AddTool(mcp.NewTool("publish_note",
			mcp.WithDescription("Publish a short note event to Nostr with the given text content"),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Arbitrary string to be published"),
			),
			mcp.WithString("mention",
				mcp.Required(),
				mcp.Description("Nostr user's public key to be mentioned"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			content, _ := request.Params.Arguments["content"].(string)
			mention, _ := request.Params.Arguments["mention"].(string)

			if mention != "" && !nostr.IsValidPublicKey(mention) {
				return mcp.NewToolResultError("the given mention isn't a valid public key, it must be 32 bytes hex, like the ones returned by search_profile"), nil
			}

			sk := os.Getenv("NOSTR_SECRET_KEY")
			if sk == "" {
				sk = "0000000000000000000000000000000000000000000000000000000000000001"
			}
			var relays []string

			evt := nostr.Event{
				Kind:      1,
				Tags:      nostr.Tags{{"client", "goose/nak"}},
				Content:   content,
				CreatedAt: nostr.Now(),
			}

			if mention != "" {
				evt.Tags = append(evt.Tags, nostr.Tag{"p", mention})
				// their inbox relays
				relays = sys.FetchInboxRelays(ctx, mention, 3)
			}

			evt.Sign(sk)

			// our write relays
			relays = append(relays, sys.FetchOutboxRelays(ctx, evt.PubKey, 3)...)

			if len(relays) == 0 {
				relays = []string{"nos.lol", "relay.damus.io"}
			}

			for res := range sys.Pool.PublishMany(ctx, []string{"nos.lol"}, evt) {
				if res.Error != nil {
					return mcp.NewToolResultError(
						fmt.Sprintf("there was an error publishing the event to the relay %s",
							res.RelayURL),
					), nil
				}
			}

			return mcp.NewToolResultText("event was successfully published with id " + evt.ID), nil
		})

		s.AddTool(mcp.NewTool("search_profile",
			mcp.WithDescription("Search for the public key of a Nostr user given their name"),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name to be searched"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := request.Params.Arguments["name"].(string)
			re := sys.Pool.QuerySingle(ctx, []string{"relay.nostr.band", "nostr.wine"}, nostr.Filter{Search: name, Kinds: []int{0}})
			if re == nil {
				return mcp.NewToolResultError("couldn't find anyone with that name"), nil
			}

			return mcp.NewToolResultText(re.PubKey), nil
		})

		s.AddTool(mcp.NewTool("get_outbox_relay_for_pubkey",
			mcp.WithDescription("Get the best relay from where to read notes from a specific Nostr user"),
			mcp.WithString("pubkey",
				mcp.Required(),
				mcp.Description("Public key of Nostr user we want to know the relay from where to read"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := request.Params.Arguments["name"].(string)
			re := sys.Pool.QuerySingle(ctx, []string{"relay.nostr.band", "nostr.wine"}, nostr.Filter{Search: name, Kinds: []int{0}})
			if re == nil {
				return mcp.NewToolResultError("couldn't find anyone with that name"), nil
			}

			return mcp.NewToolResultText(re.PubKey), nil
		})

		s.AddTool(mcp.NewTool("read_events_from_relay",
			mcp.WithDescription("Makes a REQ query to one relay using the specified parameters"),
			mcp.WithNumber("kind",
				mcp.Required(),
				mcp.Description("event kind number to include in the 'kinds' field"),
			),
			mcp.WithString("pubkey",
				mcp.Description("pubkey to include in the 'authors' field"),
			),
			mcp.WithNumber("limit",
				mcp.Required(),
				mcp.Description("maximum number of events to query"),
			),
			mcp.WithString("relay",
				mcp.Required(),
				mcp.Description("relay URL to send the query to"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			relay, _ := request.Params.Arguments["relay"].(string)
			limit, _ := request.Params.Arguments["limit"].(int)
			kind, _ := request.Params.Arguments["kind"].(int)
			pubkey, _ := request.Params.Arguments["pubkey"].(string)

			if pubkey != "" && !nostr.IsValidPublicKey(pubkey) {
				return mcp.NewToolResultError("the given pubkey isn't a valid public key, it must be 32 bytes hex, like the ones returned by search_profile"), nil
			}

			filter := nostr.Filter{
				Limit: limit,
				Kinds: []int{kind},
			}
			if pubkey != "" {
				filter.Authors = []string{pubkey}
			}

			events := sys.Pool.SubManyEose(ctx, []string{relay}, nostr.Filters{filter})

			results := make([]string, 0, limit)
			for ie := range events {
				results = append(results, ie.String())
			}

			return mcp.NewToolResultText(strings.Join(results, "\n\n")), nil
		})

		return server.ServeStdio(s)
	},
}
