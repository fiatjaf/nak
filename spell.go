package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"fiatjaf.com/nostr/sdk/hints"
	"github.com/fatih/color"
	"github.com/markusmobius/go-dateparser"
	"github.com/urfave/cli/v3"
)

var spell = &cli.Command{
	Name:        "spell",
	Usage:       "downloads a spell event and executes its REQ request",
	ArgsUsage:   "[nevent_code]",
	Description: `fetches a spell event (kind 777) and executes REQ command encoded in its tags.`,
	Flags: append(defaultKeyFlags,
		&cli.StringFlag{
			Name:  "pub",
			Usage: "public key to run spells in the context of (if you don't want to pass a --sec)",
		},
		&cli.UintFlag{
			Name:    "outbox-relays-per-pubkey",
			Aliases: []string{"n"},
			Usage:   "number of outbox relays to use for each pubkey",
			Value:   3,
		},
	),
	Action: func(ctx context.Context, c *cli.Command) error {
		configPath := c.String("config-path")
		os.MkdirAll(filepath.Join(configPath, "spells"), 0755)

		// load history from file
		var history []SpellHistoryEntry
		historyPath := filepath.Join(configPath, "spells/history")
		file, err := os.Open(historyPath)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var entry SpellHistoryEntry
				if err := json.Unmarshal([]byte(scanner.Text()), &entry); err != nil {
					continue // skip invalid entries
				}
				history = append(history, entry)
			}
		}

		if c.Args().Len() == 0 {
			// check if we have input from stdin
			for stdinEvent := range getJsonsOrBlank() {
				if stdinEvent == "{}" {
					break
				}

				var spell nostr.Event
				if err := json.Unmarshal([]byte(stdinEvent), &spell); err != nil {
					return fmt.Errorf("failed to parse spell event from stdin: %w", err)
				}
				if spell.Kind != 777 {
					return fmt.Errorf("event is not a spell (expected kind 777, got %d)", spell.Kind)
				}

				return runSpell(ctx, c, historyPath, history, nostr.EventPointer{ID: spell.ID}, spell)
			}

			// no stdin input, show recent spells
			log("recent spells:\n")
			for i, entry := range history {
				if i >= 10 {
					break
				}

				displayName := entry.Name
				if displayName == "" {
					displayName = entry.Content
					if len(displayName) > 28 {
						displayName = displayName[:27] + "…"
					}
				}
				if displayName != "" {
					displayName = color.HiMagentaString(displayName) + ": "
				}

				desc := entry.Content
				if len(desc) > 50 {
					desc = desc[0:49] + "…"
				}

				lastUsed := entry.LastUsed.Format("2006-01-02 15:04")
				stdout(fmt.Sprintf("  %s %s%s - %s",
					color.BlueString(entry.Identifier),
					displayName,
					color.YellowString(lastUsed),
					desc,
				))
			}

			return nil
		}

		// decode nevent to get the spell event
		var pointer nostr.EventPointer
		identifier := c.Args().First()
		prefix, value, err := nip19.Decode(identifier)
		if err == nil {
			if prefix != "nevent" {
				return fmt.Errorf("expected nevent code, got %s", prefix)
			}
			pointer = value.(nostr.EventPointer)
		} else {
			// search our history
			for _, entry := range history {
				if entry.Identifier == identifier || entry.Name == identifier {
					pointer = entry.Pointer
					break
				}
			}
		}

		if pointer.ID == nostr.ZeroID {
			return fmt.Errorf("invalid spell reference")
		}

		// first try to fetch spell from sys.Store
		var spell nostr.Event
		found := false
		for evt := range sys.Store.QueryEvents(nostr.Filter{IDs: []nostr.ID{pointer.ID}}, 1) {
			spell = evt
			found = true
			break
		}

		var relays []string
		if !found {
			// if not found in store, fetch from external relays
			relays = pointer.Relays
			if pointer.Author != nostr.ZeroPK {
				for _, url := range relays {
					sys.Hints.Save(pointer.Author, nostr.NormalizeURL(url), hints.LastInHint, nostr.Now())
				}
				relays = append(relays, sys.FetchOutboxRelays(ctx, pointer.Author, 3)...)
			}
			result := sys.Pool.QuerySingle(ctx, relays, nostr.Filter{IDs: []nostr.ID{pointer.ID}},
				nostr.SubscriptionOptions{Label: "nak-spell-f"})
			if result == nil {
				return fmt.Errorf("spell event not found")
			}
			spell = result.Event
		}
		if spell.Kind != 777 {
			return fmt.Errorf("event is not a spell (expected kind 777, got %d)", spell.Kind)
		}

		return runSpell(ctx, c, historyPath, history, pointer, spell)
	},
}

func runSpell(
	ctx context.Context,
	c *cli.Command,
	historyPath string,
	history []SpellHistoryEntry,
	pointer nostr.EventPointer,
	spell nostr.Event,
) error {
	// parse spell tags to build REQ filter
	spellFilter, err := buildSpellReq(ctx, c, spell.Tags)
	if err != nil {
		return fmt.Errorf("failed to parse spell tags: %w", err)
	}

	// determine relays to query
	var spellRelays []string
	var outbox bool
	relaysTag := spell.Tags.Find("relays")
	if relaysTag == nil {
		// if this tag doesn't exist assume $outbox
		relaysTag = nostr.Tag{"relays", "$outbox"}
	}
	for i := 1; i < len(relaysTag); i++ {
		switch relaysTag[i] {
		case "$outbox":
			outbox = true
		default:
			spellRelays = append(spellRelays, relaysTag[i])
		}
	}

	stream := !spell.Tags.Has("close-on-eose")

	// fill in the author if we didn't have it
	pointer.Author = spell.PubKey

	// save spell to sys.Store
	if err := sys.Store.SaveEvent(spell); err != nil {
		logverbose("failed to save spell to store: %v\n", err)
	}

	// add to history before execution
	{
		idStr := nip19.EncodeNevent(spell.ID, nil, nostr.ZeroPK)
		identifier := "spell" + idStr[len(idStr)-7:]
		nameTag := spell.Tags.Find("name")
		var name string
		if nameTag != nil {
			name = nameTag[1]
		}
		if len(history) > 100 {
			history = history[:100]
		}
		// write back to file
		file, err := os.Create(historyPath)
		if err != nil {
			return err
		}
		data, _ := json.Marshal(SpellHistoryEntry{
			Identifier: identifier,
			Name:       name,
			Content:    spell.Content,
			LastUsed:   time.Now(),
			Pointer:    pointer,
		})
		file.Write(data)
		file.Write([]byte{'\n'})
		for i, entry := range history {
			if entry.Identifier == identifier {
				continue
			}

			data, _ := json.Marshal(entry)
			file.Write(data)
			file.Write([]byte{'\n'})

			// limit history size (keep last 100)
			if i == 100 {
				break
			}
		}
		file.Close()

		logverbose("executing %s: %s relays=%v outbox=%v stream=%v\n",
			identifier, spellFilter, spellRelays, outbox, stream)
	}

	// execute
	logSpellDetails(spell)
	performReq(ctx, spellFilter, spellRelays, stream, outbox, c.Uint("outbox-relays-per-pubkey"), false, 0, "nak-spell")

	return nil
}

func buildSpellReq(ctx context.Context, c *cli.Command, tags nostr.Tags) (nostr.Filter, error) {
	filter := nostr.Filter{}

	getMe := func() (nostr.PubKey, error) {
		if !c.IsSet("sec") && !c.IsSet("prompt-sec") && c.IsSet("pub") {
			return parsePubKey(c.String("pub"))
		}

		kr, _, err := gatherKeyerFromArguments(ctx, c)
		if err != nil {
			return nostr.ZeroPK, fmt.Errorf("failed to get keyer: %w", err)
		}

		pubkey, err := kr.GetPublicKey(ctx)
		if err != nil {
			return nostr.ZeroPK, fmt.Errorf("failed to get public key from keyer: %w", err)
		}

		return pubkey, nil
	}

	for _, tag := range tags {
		if len(tag) == 0 {
			continue
		}

		switch tag[0] {
		case "cmd":
			if len(tag) < 2 || tag[1] != "REQ" {
				return nostr.Filter{}, fmt.Errorf("only REQ commands are supported")
			}

		case "k":
			for i := 1; i < len(tag); i++ {
				if kind, err := strconv.Atoi(tag[i]); err == nil {
					filter.Kinds = append(filter.Kinds, nostr.Kind(kind))
				}
			}

		case "authors":
			for i := 1; i < len(tag); i++ {
				switch tag[i] {
				case "$me":
					me, err := getMe()
					if err != nil {
						return nostr.Filter{}, err
					}
					filter.Authors = append(filter.Authors, me)
				case "$contacts":
					me, err := getMe()
					if err != nil {
						return nostr.Filter{}, err
					}
					for _, f := range sys.FetchFollowList(ctx, me).Items {
						filter.Authors = append(filter.Authors, f.Pubkey)
					}
				default:
					pubkey, err := nostr.PubKeyFromHex(tag[i])
					if err != nil {
						return nostr.Filter{}, fmt.Errorf("invalid pubkey '%s' in 'authors': %w", tag[i], err)
					}
					filter.Authors = append(filter.Authors, pubkey)
				}
			}

		case "ids":
			for i := 1; i < len(tag); i++ {
				id, err := nostr.IDFromHex(tag[i])
				if err != nil {
					return nostr.Filter{}, fmt.Errorf("invalid  id '%s' in 'authors': %w", tag[i], err)
				}
				filter.IDs = append(filter.IDs, id)
			}

		case "tag":
			if len(tag) < 3 {
				continue
			}
			tagName := tag[1]
			if filter.Tags == nil {
				filter.Tags = make(nostr.TagMap)
			}
			for i := 2; i < len(tag); i++ {
				switch tag[i] {
				case "$me":
					me, err := getMe()
					if err != nil {
						return nostr.Filter{}, err
					}
					filter.Tags[tagName] = append(filter.Tags[tagName], me.Hex())
				case "$contacts":
					me, err := getMe()
					if err != nil {
						return nostr.Filter{}, err
					}
					for _, f := range sys.FetchFollowList(ctx, me).Items {
						filter.Tags[tagName] = append(filter.Tags[tagName], f.Pubkey.Hex())
					}
				default:
					filter.Tags[tagName] = append(filter.Tags[tagName], tag[i])
				}
			}

		case "limit":
			if len(tag) >= 2 {
				if limit, err := strconv.Atoi(tag[1]); err == nil {
					filter.Limit = limit
				}
			}

		case "since":
			if len(tag) >= 2 {
				date, err := dateparser.Parse(&dateparser.Configuration{
					DefaultTimezone: time.Local,
					CurrentTime:     time.Now(),
				}, tag[1])
				if err != nil {
					return nostr.Filter{}, fmt.Errorf("invalid date %s: %w", tag[1], err)
				}
				filter.Since = nostr.Timestamp(date.Time.Unix())
			}

		case "until":
			if len(tag) >= 2 {
				date, err := dateparser.Parse(&dateparser.Configuration{
					DefaultTimezone: time.Local,
					CurrentTime:     time.Now(),
				}, tag[1])
				if err != nil {
					return nostr.Filter{}, fmt.Errorf("invalid date %s: %w", tag[1], err)
				}
				filter.Until = nostr.Timestamp(date.Time.Unix())
			}

		case "search":
			if len(tag) >= 2 {
				filter.Search = tag[1]
			}
		}
	}

	return filter, nil
}

func parseRelativeTime(timeStr string) (nostr.Timestamp, error) {
	// Handle special cases
	switch timeStr {
	case "now":
		return nostr.Now(), nil
	}

	// Try to parse as relative time (e.g., "7d", "1h", "30m")
	if strings.HasSuffix(timeStr, "d") {
		days := strings.TrimSuffix(timeStr, "d")
		if daysInt, err := strconv.Atoi(days); err == nil {
			return nostr.Now() - nostr.Timestamp(daysInt*24*60*60), nil
		}
	} else if strings.HasSuffix(timeStr, "h") {
		hours := strings.TrimSuffix(timeStr, "h")
		if hoursInt, err := strconv.Atoi(hours); err == nil {
			return nostr.Now() - nostr.Timestamp(hoursInt*60*60), nil
		}
	} else if strings.HasSuffix(timeStr, "m") {
		minutes := strings.TrimSuffix(timeStr, "m")
		if minutesInt, err := strconv.Atoi(minutes); err == nil {
			return nostr.Now() - nostr.Timestamp(minutesInt*60), nil
		}
	}

	// try to parse as direct timestamp
	if ts, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
		return nostr.Timestamp(ts), nil
	}

	return 0, fmt.Errorf("invalid time format: %s", timeStr)
}

type SpellHistoryEntry struct {
	Identifier string             `json:"_id"`
	Name       string             `json:"name,omitempty"`
	Content    string             `json:"content,omitempty"`
	LastUsed   time.Time          `json:"last_used"`
	Pointer    nostr.EventPointer `json:"pointer"`
}

func logSpellDetails(spell nostr.Event) {
	nameTag := spell.Tags.Find("name")
	name := ""
	if nameTag != nil {
		name = nameTag[1]
		if len(name) > 28 {
			name = name[:27] + "…"
		}
	}
	if name != "" {
		name = ": " + color.HiMagentaString(name)
	}

	desc := spell.Content
	if len(desc) > 50 {
		desc = desc[0:49] + "…"
	}

	idStr := nip19.EncodeNevent(spell.ID, nil, nostr.ZeroPK)
	identifier := "spell" + idStr[len(idStr)-7:]

	log("running %s%s - %s\n",
		color.BlueString(identifier),
		name,
		desc,
	)
}

func createSpellEvent(ctx context.Context, filter nostr.Filter, kr nostr.Keyer) nostr.Event {
	spell := nostr.Event{
		Kind: 777,
		Tags: make(nostr.Tags, 0),
	}

	// add cmd tag
	spell.Tags = append(spell.Tags, nostr.Tag{"cmd", "REQ"})

	// add kinds
	if len(filter.Kinds) > 0 {
		kindTag := nostr.Tag{"k"}
		for _, kind := range filter.Kinds {
			kindTag = append(kindTag, strconv.Itoa(int(kind)))
		}
		spell.Tags = append(spell.Tags, kindTag)
	}

	// add authors
	if len(filter.Authors) > 0 {
		authorsTag := nostr.Tag{"authors"}
		for _, author := range filter.Authors {
			authorsTag = append(authorsTag, author.Hex())
		}
		spell.Tags = append(spell.Tags, authorsTag)
	}

	// add ids
	if len(filter.IDs) > 0 {
		idsTag := nostr.Tag{"ids"}
		for _, id := range filter.IDs {
			idsTag = append(idsTag, id.Hex())
		}
		spell.Tags = append(spell.Tags, idsTag)
	}

	// add tags
	for tagName, values := range filter.Tags {
		if len(values) > 0 {
			tag := nostr.Tag{"tag", tagName}
			for _, value := range values {
				tag = append(tag, value)
			}
			spell.Tags = append(spell.Tags, tag)
		}
	}

	// add limit
	if filter.Limit > 0 {
		spell.Tags = append(spell.Tags, nostr.Tag{"limit", strconv.Itoa(filter.Limit)})
	}

	// add since
	if filter.Since > 0 {
		spell.Tags = append(spell.Tags, nostr.Tag{"since", strconv.FormatInt(int64(filter.Since), 10)})
	}

	// add until
	if filter.Until > 0 {
		spell.Tags = append(spell.Tags, nostr.Tag{"until", strconv.FormatInt(int64(filter.Until), 10)})
	}

	// add search
	if filter.Search != "" {
		spell.Tags = append(spell.Tags, nostr.Tag{"search", filter.Search})
	}

	if err := kr.SignEvent(ctx, &spell); err != nil {
		log("failed to sign spell: %s\n", err)
	}

	return spell
}
