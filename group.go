package main

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip11"
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

				group, err := fetchGroupMetadata(ctx, relay, identifier)
				if err != nil {
					return err
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
				stdout("livekit:",
					color.HiBlueString("%s", cond(group.LiveKit, "yes", "no"))+
						", "+
						cond(group.LiveKit, "group supports live audio/video with livekit", "group has no advertised live audio/video support"),
				)
				supportedKinds := "unspecified"
				if group.SupportedKinds != nil {
					if len(group.SupportedKinds) == 0 {
						supportedKinds = "none"
					} else {
						kinds := make([]string, 0, len(group.SupportedKinds))
						for _, kind := range group.SupportedKinds {
							kinds = append(kinds, strconv.Itoa(int(kind)))
						}
						supportedKinds = strings.Join(kinds, ", ")
					}
				}
				stdout("supported-kinds:", color.HiBlueString(supportedKinds))
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
					ArgsUsage: "<relay>'<identifier> <message>",
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
			Name:        "talk",
			Usage:       "get livekit connection details",
			Description: "requests a livekit jwt for this group and prints the livekit server url.",
			ArgsUsage:   "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				group, err := fetchGroupMetadata(ctx, relay, identifier)
				if err != nil {
					return err
				}
				if !group.LiveKit {
					return fmt.Errorf("group doesn't advertise livekit support")
				}

				serverURL, jwt, err := requestLivekitJWT(ctx, c, relay, identifier)
				if err != nil {
					return err
				}

				stdout("livekit:", color.HiBlueString(serverURL))
				stdout("jwt:", color.HiBlueString(jwt))
				stdout("join:", color.HiBlueString(
					fmt.Sprintf("https://meet.livekit.io/custom?liveKitUrl=%s&token=%s", serverURL, jwt)),
				)
				return nil
			},
		},
		{
			Name:        "forum",
			Usage:       "forum topic operations",
			Description: "when called directly, lists forum topics; with an id prefix, displays that topic with threaded comments.",
			ArgsUsage:   "<relay>'<identifier> [id-prefix]",
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				topics, err := fetchGroupForumTopics(ctx, relay, identifier)
				if err != nil {
					return err
				}

				prefix := strings.TrimSpace(c.Args().Get(1))
				if prefix == "" {
					if len(topics) == 0 {
						log("no forum topics found\n")
						return nil
					}

					wg := sync.WaitGroup{}
					for _, evt := range topics {
						wg.Go(func() {
							sys.FetchProfileMetadata(ctx, evt.PubKey)
						})
					}
					wg.Wait()

					for _, evt := range topics {
						id := evt.ID.Hex()
						date := evt.CreatedAt.Time().Format(time.DateOnly)
						author := authorPreview(ctx, evt.PubKey)
						subject := forumSubjectPreview(evt, 72)
						if subject == "" {
							subject = "<untitled>"
						}
						stdout(color.CyanString(id[:6]), color.HiBlackString(date), color.HiBlueString(author), color.HiWhiteString(subject))
					}

					return nil
				}

				evt, err := findEventByPrefix(topics, prefix)
				if err != nil {
					return err
				}

				return showThreadWithComments(ctx, []string{relay}, evt, "", nostr.TagMap{"h": []string{identifier}})
			},
			Commands: []*cli.Command{
				{
					Name:  "create",
					Usage: "edit and send a forum topic event (kind 11)",
					Action: func(ctx context.Context, c *cli.Command) error {
						relay, identifier, err := parseGroupIdentifier(c)
						if err != nil {
							return err
						}

						groupMeta, err := fetchGroupMetadata(ctx, relay, identifier)
						if err != nil {
							return err
						}
						groupName := groupMeta.Name
						if groupName == "" {
							groupName = identifier
						}

						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						_, selfName, selfNpub, err := keyerIdentity(ctx, kr)
						if err != nil {
							return fmt.Errorf("failed to get current identity: %w", err)
						}

						content, err := editWithDefaultEditor(
							"nak-group-forum/NOTES_EDITMSG",
							strings.TrimSpace(fmt.Sprintf(`# creating as '%s' ('%s')
# creating forum topic in group '%s' ('%s''%s')
# the first line will be used as the topic title
topic title here

# the remaining lines will be the body
write your forum post

# lines starting with '#' are ignored
`, selfName, selfNpub, groupName, relay, identifier)),
							true,
						)
						if err != nil {
							return err
						}

						title, body, err := parseForumCreateContent(content)
						if err != nil {
							return err
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      11,
							Tags: nostr.Tags{
								nostr.Tag{"h", identifier},
								nostr.Tag{"title", title},
							},
							Content: body,
						}
						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign forum topic event: %w", err)
						}

						r, err := sys.Pool.EnsureRelay(relay)
						if err != nil {
							return err
						}

						return r.Publish(ctx, evt)
					},
				},
				{
					Name:      "comment",
					Usage:     "comment on a forum topic with a NIP-22 comment event",
					ArgsUsage: "<relay>'<identifier> <id-prefix>",
					Action: func(ctx context.Context, c *cli.Command) error {
						relay, identifier, err := parseGroupIdentifier(c)
						if err != nil {
							return err
						}

						prefix := strings.TrimSpace(c.Args().Get(1))
						if prefix == "" {
							return fmt.Errorf("missing forum topic id prefix")
						}

						kr, _, err := gatherKeyerFromArguments(ctx, c)
						if err != nil {
							return fmt.Errorf("failed to gather keyer: %w", err)
						}

						_, selfName, selfNpub, err := keyerIdentity(ctx, kr)
						if err != nil {
							return fmt.Errorf("failed to get current identity: %w", err)
						}

						topics, err := fetchGroupForumTopics(ctx, relay, identifier)
						if err != nil {
							return err
						}

						topic, err := findEventByPrefix(topics, prefix)
						if err != nil {
							return err
						}

						groupMeta, err := fetchGroupMetadata(ctx, relay, identifier)
						if err != nil {
							return err
						}
						groupName := groupMeta.Name
						if groupName == "" {
							groupName = identifier
						}

						subject := forumSubjectPreview(topic, 72)
						if subject == "" {
							subject = "<untitled>"
						}
						pm := sys.FetchProfileMetadata(ctx, topic.PubKey)
						headerLines := []string{
							fmt.Sprintf("commenting as '%s' ('%s')", selfName, selfNpub),
							fmt.Sprintf("commenting on forum topic '%s' '%s' by '%s' ('%s') in group '%s' ('%s''%s')", topic.ID.Hex()[:6], subject, pm.ShortName(), pm.NpubShort(), groupName, relay, identifier),
						}

						comments, err := fetchThreadComments(ctx, []string{relay}, topic.ID, nostr.TagMap{"h": []string{identifier}})
						if err != nil {
							return err
						}

						edited, err := editWithDefaultEditor(
							"nak-group-forum-reply/NOTES_EDITMSG",
							threadReplyEditorTemplate(ctx, headerLines, topic, comments),
							true,
						)
						if err != nil {
							return err
						}

						content, parentEvt, err := parseThreadReplyContent(topic, comments, edited)
						if err != nil {
							return err
						}

						rootRelay := relay
						if topic.Relay.URL != "" {
							rootRelay = topic.Relay.URL
						}
						parentRelay := rootRelay
						if parentEvt.Relay.URL != "" {
							parentRelay = parentEvt.Relay.URL
						}

						evt := nostr.Event{
							CreatedAt: nostr.Now(),
							Kind:      1111,
							Tags: nostr.Tags{
								nostr.Tag{"E", topic.ID.Hex(), rootRelay},
								nostr.Tag{"e", parentEvt.ID.Hex(), parentRelay},
								nostr.Tag{"P", topic.PubKey.Hex()},
								nostr.Tag{"p", parentEvt.PubKey.Hex()},
								nostr.Tag{"h", identifier},
							},
							Content: content,
						}
						if err := kr.SignEvent(ctx, &evt); err != nil {
							return fmt.Errorf("failed to sign forum comment event: %w", err)
						}

						r, err := sys.Pool.EnsureRelay(relay)
						if err != nil {
							return err
						}

						return r.Publish(ctx, evt)
					},
				},
			},
		},
		{
			Name:      "put-user",
			Usage:     "add a user to the group with optional roles",
			ArgsUsage: "<relay>'<identifier>",
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:     "pubkey",
					Required: true,
				},
				&cli.StringSliceFlag{
					Name: "role",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				return createModerationEvent(ctx, c, 9000, func(evt *nostr.Event, args []string) error {
					pubkey := getPubKey(c, "pubkey")
					tag := nostr.Tag{"p", pubkey.Hex()}
					tag = append(tag, c.StringSlice("role")...)
					evt.Tags = append(evt.Tags, tag)
					return nil
				})
			},
		},
		{
			Name:      "remove-user",
			Usage:     "remove a user from the group",
			ArgsUsage: "<relay>'<identifier> <pubkey>",
			Flags: []cli.Flag{
				&PubKeyFlag{
					Name:     "pubkey",
					Required: true,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				return createModerationEvent(ctx, c, 9001, func(evt *nostr.Event, args []string) error {
					pubkey := getPubKey(c, "pubkey")
					evt.Tags = append(evt.Tags, nostr.Tag{"p", pubkey.Hex()})
					return nil
				})
			},
		},
		{
			Name:      "edit-metadata",
			Usage:     "edits the group metadata",
			ArgsUsage: "<relay>'<identifier>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name: "name",
				},
				&cli.StringFlag{
					Name: "about",
				},
				&cli.StringFlag{
					Name: "picture",
				},
				&cli.BoolFlag{
					Name: "restricted",
				},
				&cli.BoolFlag{
					Name: "unrestricted",
				},
				&cli.BoolFlag{
					Name: "closed",
				},
				&cli.BoolFlag{
					Name: "open",
				},
				&cli.BoolFlag{
					Name: "hidden",
				},
				&cli.BoolFlag{
					Name: "visible",
				},
				&cli.BoolFlag{
					Name: "private",
				},
				&cli.BoolFlag{
					Name: "public",
				},
				&cli.BoolFlag{
					Name: "livekit",
				},
				&cli.BoolFlag{
					Name: "no-livekit",
				},
				&cli.IntSliceFlag{
					Name:    "kind",
					Aliases: []string{"supported-kinds"},
					Usage:   "list of event kind numbers supported by this group",
				},
				&cli.BoolFlag{
					Name:  "all-kinds",
					Usage: "specify this to delete the supported_kinds property, meaning everything will be supported",
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				relay, identifier, err := parseGroupIdentifier(c)
				if err != nil {
					return err
				}

				if c.Bool("livekit") || c.Bool("no-livekit") {
					if err := checkRelayLivekitMetadataSupport(ctx, relay); err != nil {
						return err
					}
				}

				group, err := fetchGroupMetadata(ctx, relay, identifier)
				if err != nil {
					return err
				}
				if group.Name == "" {
					group.Name = identifier
				}

				if name := c.String("name"); name != "" {
					group.Name = name
				}
				if picture := c.String("picture"); picture != "" {
					group.Picture = picture
				}
				if about := c.String("about"); about != "" {
					group.About = about
				}
				if c.Bool("restricted") {
					group.Restricted = true
				} else if c.Bool("unrestricted") {
					group.Restricted = false
				}
				if c.Bool("closed") {
					group.Closed = true
				} else if c.Bool("open") {
					group.Closed = false
				}
				if c.Bool("hidden") {
					group.Hidden = true
				} else if c.Bool("visible") {
					group.Hidden = false
				}
				if c.Bool("private") {
					group.Private = true
				} else if c.Bool("public") {
					group.Private = false
				}
				if c.Bool("livekit") {
					group.LiveKit = true
				} else if c.Bool("no-livekit") {
					group.LiveKit = false
				}
				if supportedKinds := c.IntSlice("kind"); len(supportedKinds) > 0 {
					kinds := make([]nostr.Kind, 0, len(supportedKinds))
					for _, kind := range supportedKinds {
						kinds = append(kinds, nostr.Kind(kind))
					}
					group.SupportedKinds = kinds
				} else if c.Bool("all-kinds") {
					group.SupportedKinds = nil
				}

				return createModerationEvent(ctx, c, 9002, func(evt *nostr.Event, args []string) error {
					evt.Tags = append(evt.Tags, nostr.Tag{"name", group.Name})
					evt.Tags = append(evt.Tags, nostr.Tag{"picture", group.Picture})
					evt.Tags = append(evt.Tags, nostr.Tag{"about", group.About})
					if group.Restricted {
						evt.Tags = append(evt.Tags, nostr.Tag{"restricted"})
					} else {
						evt.Tags = append(evt.Tags, nostr.Tag{"unrestricted"})
					}
					if group.Closed {
						evt.Tags = append(evt.Tags, nostr.Tag{"closed"})
					} else {
						evt.Tags = append(evt.Tags, nostr.Tag{"open"})
					}
					if group.Hidden {
						evt.Tags = append(evt.Tags, nostr.Tag{"hidden"})
					} else {
						evt.Tags = append(evt.Tags, nostr.Tag{"visible"})
					}
					if group.Private {
						evt.Tags = append(evt.Tags, nostr.Tag{"private"})
					} else {
						evt.Tags = append(evt.Tags, nostr.Tag{"public"})
					}
					if group.LiveKit {
						evt.Tags = append(evt.Tags, nostr.Tag{"livekit"})
					} else {
						evt.Tags = append(evt.Tags, nostr.Tag{"no-livekit"})
					}
					if group.SupportedKinds != nil {
						tag := make(nostr.Tag, 1, 1+len(group.SupportedKinds))
						tag[0] = "supported_kinds"
						for _, kind := range group.SupportedKinds {
							tag = append(tag, strconv.Itoa(int(kind)))
						}
						evt.Tags = append(evt.Tags, tag)
					}
					return nil
				})
			},
		},
		{
			Name:      "delete-event",
			Usage:     "delete an event from the group",
			ArgsUsage: "<relay>'<identifier>",
			Flags: []cli.Flag{
				&IDFlag{
					Name:     "event",
					Required: true,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				return createModerationEvent(ctx, c, 9005, func(evt *nostr.Event, args []string) error {
					id := getID(c, "event")
					evt.Tags = append(evt.Tags, nostr.Tag{"e", id.Hex()})
					return nil
				})
			},
		},
		{
			Name:      "delete-group",
			Usage:     "deletes the group",
			ArgsUsage: "<relay>'<identifier>",
			Action: func(ctx context.Context, c *cli.Command) error {
				return createModerationEvent(ctx, c, 9008, func(evt *nostr.Event, args []string) error {
					return nil
				})
			},
		},
		{
			Name:      "create-invite",
			Usage:     "creates an invite code",
			ArgsUsage: "<relay>'<identifier>",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "code",
					Required: true,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) error {
				return createModerationEvent(ctx, c, 9009, func(evt *nostr.Event, args []string) error {
					evt.Tags = append(evt.Tags, nostr.Tag{"code", c.String("code")})
					return nil
				})
			},
		},
	},
}

func createModerationEvent(ctx context.Context, c *cli.Command, kind nostr.Kind, setupFunc func(*nostr.Event, []string) error) error {
	args := c.Args().Slice()
	if len(args) < 1 {
		return fmt.Errorf("requires group identifier")
	}

	relay, identifier, err := parseGroupIdentifier(c)
	if err != nil {
		return err
	}

	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return err
	}

	evt := nostr.Event{
		Kind:      kind,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"h", identifier},
		},
	}

	if err := setupFunc(&evt, args); err != nil {
		return err
	}

	if err := kr.SignEvent(ctx, &evt); err != nil {
		return fmt.Errorf("failed to sign event: %w", err)
	}

	stdout(evt.String())

	r, err := sys.Pool.EnsureRelay(relay)
	if err != nil {
		return err
	}

	return r.Publish(ctx, evt)
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

func fetchGroupMetadata(ctx context.Context, relay string, identifier string) (nip29.Group, error) {
	group := nip29.Group{}

	filter := nostr.Filter{
		Kinds: []nostr.Kind{nostr.KindSimpleGroupMetadata},
		Tags:  nostr.TagMap{"d": []string{identifier}},
		Limit: 1,
	}

	if info, err := nip11.Fetch(ctx, relay); err == nil {
		if info.Self != nil {
			filter.Authors = append(filter.Authors, *info.Self)
		} else if info.PubKey != nil {
			filter.Authors = append(filter.Authors, *info.PubKey)
		}
	}

	for ie := range sys.Pool.FetchMany(ctx, []string{relay}, filter, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
		if err := group.MergeInMetadataEvent(&ie.Event); err != nil {
			return group, err
		}

		break
	}

	return group, nil
}

func fetchGroupForumTopics(ctx context.Context, relay string, identifier string) ([]nostr.RelayEvent, error) {
	topics := make([]nostr.RelayEvent, 0, 30)
	for ie := range sys.Pool.FetchMany(ctx, []string{relay}, nostr.Filter{
		Kinds: []nostr.Kind{11},
		Tags:  nostr.TagMap{"h": []string{identifier}},
		Limit: 500,
	}, nostr.SubscriptionOptions{Label: "nak-nip29"}) {
		topics = append(topics, ie)
	}

	slices.SortFunc(topics, nostr.CompareRelayEvent)
	return topics, nil
}

func forumSubjectPreview(evt nostr.RelayEvent, maxChars int) string {
	if tag := evt.Tags.Find("title"); len(tag) >= 2 {
		subject := strings.TrimSpace(tag[1])
		if subject != "" {
			return clampWithEllipsis(subject, maxChars)
		}
	}

	if tag := evt.Tags.Find("subject"); len(tag) >= 2 {
		subject := strings.TrimSpace(tag[1])
		if subject != "" {
			return clampWithEllipsis(subject, maxChars)
		}
	}

	for _, line := range strings.Split(evt.Content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return clampWithEllipsis(line, maxChars)
		}
	}

	return ""
}

func parseForumCreateContent(content string) (title string, body string, err error) {
	lines := strings.Split(content, "\n")
	var bodyb strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if title == "" {
			title = line
			continue
		}

		bodyb.WriteString(line)
		bodyb.WriteByte('\n')
	}

	if title == "" {
		return "", "", fmt.Errorf("topic title cannot be empty")
	}

	body = strings.TrimSpace(bodyb.String())
	return title, body, nil
}

func checkRelayLivekitMetadataSupport(ctx context.Context, relay string) error {
	url := "http" + nostr.NormalizeURL(relay)[2:] + "/.well-known/nip29/livekit"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create livekit support request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check relay livekit support: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("relay doesn't advertise livekit support at %s (expected 204, got %d)", url, resp.StatusCode)
	}

	return nil
}

func requestLivekitJWT(ctx context.Context, c *cli.Command, relay string, identifier string) (serverURL string, jwt string, err error) {
	kr, _, err := gatherKeyerFromArguments(ctx, c)
	if err != nil {
		return "", "", err
	}

	url := "http" + nostr.NormalizeURL(relay)[2:] + "/.well-known/nip29/livekit/" + identifier
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create livekit token request: %w", err)
	}

	tokenEvent := nostr.Event{
		Kind:      27235,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"u", url},
			{"method", "GET"},
		},
	}
	if err := kr.SignEvent(ctx, &tokenEvent); err != nil {
		return "", "", fmt.Errorf("failed to sign livekit auth token: %w", err)
	}

	evtj, _ := stdjson.Marshal(tokenEvent)
	req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(evtj))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("livekit token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed reading livekit token response: %w", err)
	}

	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			return "", "", fmt.Errorf("livekit token request failed with status %d: %s", resp.StatusCode, msg)
		}
		return "", "", fmt.Errorf("livekit token request failed with status %d", resp.StatusCode)
	}

	response := struct {
		ServerURL        string `json:"server_url"`
		ParticipantToken string `json:"participant_token"`
	}{}
	if err := stdjson.Unmarshal(body, &response); err != nil {
		return "", "", fmt.Errorf("invalid livekit token response: %w", err)
	}

	serverURL = response.ServerURL
	jwt = response.ParticipantToken

	if serverURL == "" || jwt == "" {
		return "", "", fmt.Errorf("livekit token response missing url or jwt")
	}

	return serverURL, jwt, nil
}
