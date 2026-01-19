package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip29"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var group = &cli.Command{
	Name:                      "group",
	Aliases:                   []string{"nip29"},
	Usage:                     "group-related operations: info, chat, forum, members, admins, roles",
	Description:               `manage and interact with Nostr communities (NIP-29). Use "nak group <subcommand> <relay>'<identifier>" where host.tld is the relay and identifier is the group identifier.`,
	DisableSliceFlagSeparator: true,
	ArgsUsage:                 "<subcommand> <relay>'<identifier> [flags]",
	Flags:                     defaultKeyFlags,
	Commands: []*cli.Command{
		{
			Name:        "info",
			Usage:       "show group information",
			Description: "displays basic group metadata.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				group := nip29.Group{}
				for ie := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
					Kinds: []nostr.Kind{nostr.KindSimpleGroupMetadata},
					Tags:  nostr.TagMap{"d": []string{identifier}},
				}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
					if err := group.MergeInMetadataEvent(&ie.Event); err != nil {
						return err
					}
					break
				}

				stdout("address:", color.HiBlueString(strings.SplitN(nostr.NormalizeURL(relay), "/", 3)[2]+"'"+identifier))
				stdout("name:", color.HiBlueString(group.Name))
				stdout("picture:", color.HiBlueString(group.Picture))
				stdout("about:", color.HiBlueString(group.About))
				stdout("restricted:",
					color.HiBlueString("%s", cond(group.Restricted, "yes", "no"))+
						", "+
						cond(group.Restricted, "only explicit members can publish", "non-members can publish (restricted by relay policy)"),
				)
				stdout("closed:",
					color.HiBlueString("%s", cond(group.Closed, "yes", "no"))+
						", "+
						cond(group.Closed, "joining requires an invite", "anyone can join (restricted by relay policy)"),
				)
				stdout("hidden:",
					color.HiBlueString("%s", cond(group.Hidden, "yes", "no"))+
						", "+
						cond(group.Hidden, "group doesn't show up when listing relay groups", "group is visible to users browsing the relay"),
				)
				stdout("private:",
					color.HiBlueString("%s", cond(group.Private, "yes", "no"))+
						", "+
						cond(group.Private, "group content is not accessible to non-members", "group content is public"),
				)
				return nil
			},
		},
		{
			Name:        "members",
			Usage:       "list and manage group members",
			Description: "view group membership information.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				group := nip29.Group{
					Members: make(map[nostr.PubKey][]*nip29.Role),
				}
				for ie := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
					Kinds: []nostr.Kind{nostr.KindSimpleGroupMembers},
					Tags:  nostr.TagMap{"d": []string{identifier}},
				}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
					if err := group.MergeInMembersEvent(&ie.Event); err != nil {
						return err
					}
					break
				}

				lines := make(chan string)
				wg := sync.WaitGroup{}

				for member, roles := range group.Members {
					wg.Go(func() {
						line := member.Hex()

						meta := sys.FetchProfileMetadata(ctx, member)
						line += " (" + color.HiBlueString(meta.ShortName()) + ")"

						for _, role := range roles {
							line += ", " + role.Name
						}

						lines <- line
					})
				}

				go func() {
					wg.Wait()
					close(lines)
				}()

				for line := range lines {
					stdout(line)
				}

				return nil
			},
		},
		{
			Name:        "admins",
			Usage:       "manage group administrators",
			Description: "view and manage group admin permissions.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				group := nip29.Group{
					Members: make(map[nostr.PubKey][]*nip29.Role),
				}
				for ie := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
					Kinds: []nostr.Kind{nostr.KindSimpleGroupAdmins},
					Tags:  nostr.TagMap{"d": []string{identifier}},
				}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
					if err := group.MergeInAdminsEvent(&ie.Event); err != nil {
						return err
					}
					break
				}

				lines := make(chan string)
				wg := sync.WaitGroup{}

				for member, roles := range group.Members {
					wg.Go(func() {
						line := member.Hex()

						meta := sys.FetchProfileMetadata(ctx, member)
						line += " (" + color.HiBlueString(meta.ShortName()) + ")"

						for _, role := range roles {
							line += ", " + role.Name
						}

						lines <- line
					})
				}

				go func() {
					wg.Wait()
					close(lines)
				}()

				for line := range lines {
					stdout(line)
				}

				return nil
			},
		},
		{
			Name:        "roles",
			Usage:       "manage group roles and permissions",
			Description: "configure custom roles and permissions within the group.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				group := nip29.Group{
					Roles: make([]*nip29.Role, 0),
				}
				for ie := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
					Kinds: []nostr.Kind{nostr.KindSimpleGroupRoles},
					Tags:  nostr.TagMap{"d": []string{identifier}},
				}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
					if err := group.MergeInRolesEvent(&ie.Event); err != nil {
						return err
					}
					break
				}

				for _, role := range group.Roles {
					stdout(color.HiBlueString(role.Name) + " " + role.Description)
				}

				return nil
			},
		},
		{
			Name:        "chat",
			Usage:       "send and read group chat messages",
			Description: "interact with group chat functionality.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				r, err := sys.Pool.EnsureRelay(relay)
				if err != nil {
					return err
				}

				sub, err := r.Subscribe(ctx, nostr.Filter{
					Kinds: []nostr.Kind{9},
					Tags:  nostr.TagMap{"h": []string{identifier}},
					Limit: 200,
				}, nostr.SubscriptionOptions{Label: "nak-nip29"})
				if err != nil {
					return err
				}
				defer sub.Close()

				eosed := false
				messages := make([]struct {
					message  string
					rendered bool
				}, 200)
				base := len(messages)

				tryRender := func(i int) {
					// if all messages before these are loaded we can render this,
					// otherwise we render whatever we can and stop
					for m, msg := range messages[base:] {
						if msg.rendered {
							continue
						}
						if msg.message == "" {
							break
						}
						messages[base+m].rendered = true
						stdout(msg.message)
					}
				}

				for {
					select {
					case evt := <-sub.Events:
						var i int
						if eosed {
							i = len(messages)
							messages = append(messages, struct {
								message  string
								rendered bool
							}{})
						} else {
							base--
							i = base
						}

						go func() {
							meta := sys.FetchProfileMetadata(ctx, evt.PubKey)
							messages[i].message = color.HiBlueString(meta.ShortName()) + " " + color.HiCyanString(evt.CreatedAt.Time().Format(time.DateTime)) + ": " + evt.Content

							if eosed {
								tryRender(i)
							}
						}()
					case reason := <-sub.ClosedReason:
						stdout("closed:" + color.YellowString(reason))
					case <-sub.EndOfStoredEvents:
						eosed = true
						tryRender(len(messages) - 1)
					case <-sub.Context.Done():
						return fmt.Errorf("subscription ended: %w", context.Cause(sub.Context))
					}
				}
			},
			Commands: []*cli.Command{
				{
					Name:      "send",
					Usage:     "sends a message to the chat",
					ArgsUsage: "<message>",
					Action: func(ctx context.Context, c *cli.Command) error {
						relay, identifier, err := parseGroupIdentifier(c)
						if err != nil {
							return err
						}

						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return err
						}

						msg := nostr.Event{
							Kind:      9,
							CreatedAt: nostr.Now(),
							Content:   strings.Join(c.Args().Tail(), " "),
							Tags: nostr.Tags{
								{"h", identifier},
							},
						}
						if err := kr.SignEvent(ctx, &msg); err != nil {
							return fmt.Errorf("failed to sign message: %w", err)
						}

						if r, err := sys.Pool.EnsureRelay(relay); err != nil {
							return err
						} else {
							return r.Publish(ctx, msg)
						}
					},
				},
			},
		},
		{
			Name:        "forum",
			Usage:       "read group forum posts",
			Description: "access group forum functionality.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				for evt := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
					Kinds: []nostr.Kind{11},
					Tags:  nostr.TagMap{"#h": []string{identifier}},
				}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
					title := evt.Tags.Find("title")
					if title != nil {
						stdout(colors.bold(title[1]))
					} else {
						stdout(colors.bold("<untitled>"))
					}
					meta := sys.FetchProfileMetadata(ctx, evt.PubKey)
					stdout("by " + evt.PubKey.Hex() + " (" + color.HiBlueString(meta.ShortName()) + ") at " + evt.CreatedAt.Time().Format(time.DateTime))
					stdout(evt.Content)
				}
				// TODO: see what to do about this

				return nil
			},
		},
	},
}

func cond(b bool, ifYes string, ifNo string) string {
	if b {
		return ifYes
	}
	return ifNo
}

func parseGroupIdentifier(c *cli.Command) (relay string, identifier string, err error) {
	groupArg := c.Args().First()
	if !strings.Contains(groupArg, "'") {
		return "", "", fmt.Errorf("invalid group identifier format, expected <relay>'<identifier>")
	}

	parts := strings.SplitN(groupArg, "'", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid group identifier format, expected <relay>'<identifier>")
	}

	return strings.TrimSuffix(parts[0], "/"), parts[1], nil
}
