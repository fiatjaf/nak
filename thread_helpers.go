package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip22"
	"github.com/fatih/color"
)

func fetchThreadComments(ctx context.Context, relays []string, discussionID nostr.ID, extraTags nostr.TagMap) ([]nostr.RelayEvent, error) {
	filterTags := nostr.TagMap{
		"E": []string{discussionID.Hex()},
	}
	for key, values := range extraTags {
		filterTags[key] = values
	}

	comments := make([]nostr.RelayEvent, 0, 15)
	for ie := range sys.Pool.FetchMany(ctx, relays, nostr.Filter{
		Kinds: []nostr.Kind{1111},
		Tags:  filterTags,
		Limit: 500,
	}, nostr.SubscriptionOptions{Label: "nak-thread"}) {
		comments = append(comments, ie)
	}

	slices.SortFunc(comments, nostr.CompareRelayEvent)

	return comments, nil
}

func showThreadWithComments(
	ctx context.Context,
	relays []string,
	evt nostr.RelayEvent,
	status string,
	extraTags nostr.TagMap,
) error {
	comments, err := fetchThreadComments(ctx, relays, evt.ID, extraTags)
	if err != nil {
		return err
	}

	printThreadMetadata(ctx, os.Stdout, evt, status, true)
	stdout("")
	stdout(evt.Content)

	if len(comments) > 0 {
		stdout("")
		stdout(color.CyanString("comments:"))
		printThreadedComments(ctx, os.Stdout, comments, evt.ID, true)
	}

	return nil
}

func printThreadedComments(
	ctx context.Context,
	w io.Writer,
	comments []nostr.RelayEvent,
	discussionID nostr.ID,
	withColor bool,
) {
	byID := make(map[nostr.ID]struct{}, len(comments)+1)
	byID[discussionID] = struct{}{}
	for _, c := range comments {
		byID[c.ID] = struct{}{}
	}

	// preload metadata from everybody
	wg := sync.WaitGroup{}

	children := make(map[nostr.ID][]nostr.RelayEvent, len(comments)+1)
	for _, c := range comments {
		wg.Go(func() {
			sys.FetchProfileMetadata(ctx, c.PubKey)
		})

		parent, ok := nip22.GetImmediateParent(c.Event.Tags).(nostr.EventPointer)
		if !ok {
			continue
		}
		if _, ok := byID[parent.ID]; ok {
			children[parent.ID] = append(children[parent.ID], c)
		}
	}

	for parent := range children {
		slices.SortFunc(children[parent], nostr.CompareRelayEvent)
	}

	wg.Wait()

	var render func(parent nostr.ID, depth int)
	render = func(parent nostr.ID, depth int) {
		for _, c := range children[parent] {
			indent := strings.Repeat("  ", depth)
			author := authorPreview(ctx, c.PubKey)
			created := c.CreatedAt.Time().Format(time.DateTime)

			if withColor {
				fmt.Fprintln(w, indent+color.CyanString("["+c.ID.Hex()[0:6]+"]"), color.HiBlueString(author), color.HiBlackString(created))
			} else {
				fmt.Fprintln(w, indent+"["+c.ID.Hex()[0:6]+"] "+author+" "+created)
			}

			for _, line := range strings.Split(c.Content, "\n") {
				fmt.Fprintln(w, indent+"  "+line)
			}
			fmt.Fprintln(w, indent+"")

			render(c.ID, depth+1)
		}
	}

	render(discussionID, 0)
}

func findEventByPrefix(events []nostr.RelayEvent, prefix string) (nostr.RelayEvent, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return nostr.RelayEvent{}, fmt.Errorf("missing event id prefix")
	}

	matchCount := 0
	matched := nostr.RelayEvent{}
	for _, evt := range events {
		if strings.HasPrefix(evt.ID.Hex(), prefix) {
			matched = evt
			matchCount++
		}
	}

	if matchCount == 0 {
		return nostr.RelayEvent{}, fmt.Errorf("no event found with id prefix '%s'", prefix)
	}
	if matchCount > 1 {
		return nostr.RelayEvent{}, fmt.Errorf("id prefix '%s' is ambiguous", prefix)
	}

	return matched, nil
}

func printThreadMetadata(
	ctx context.Context,
	w io.Writer,
	evt nostr.RelayEvent,
	status string,
	withColors bool,
) {
	label := func(s string) string { return s }
	value := func(s string) string { return s }
	statusValue := func(s string) string { return s }
	if withColors {
		label = func(s string) string { return color.CyanString(s) }
		value = func(s string) string { return color.HiWhiteString(s) }
		statusValue = colorizeGitStatus
	}

	fmt.Fprintln(w, label("id:"), value(evt.ID.Hex()))
	fmt.Fprintln(w, label("kind:"), value(fmt.Sprintf("%d", evt.Kind.Num())))
	fmt.Fprintln(w, label("author:"), value(authorPreview(ctx, evt.PubKey)))
	fmt.Fprintln(w, label("created:"), value(evt.CreatedAt.Time().Format(time.RFC3339)))
	if status != "" {
		fmt.Fprintln(w, label("status:"), statusValue(status))
	}
	if subject := evt.Tags.Find("subject"); subject != nil && len(subject) >= 2 {
		fmt.Fprintln(w, label("subject:"), value(subject[1]))
		fmt.Fprintln(w, "")
	} else if title := evt.Tags.Find("title"); title != nil && len(title) >= 2 {
		fmt.Fprintln(w, label("title:"), value(title[1]))
		fmt.Fprintln(w, "")
	}
}

func parseThreadReplyContent(discussion nostr.RelayEvent, comments []nostr.RelayEvent, edited string) (string, nostr.RelayEvent, error) {
	currentParent := discussion
	selectedParent := nostr.ZeroID
	inComments := false

	replyb := strings.Builder{}
	for _, line := range strings.Split(edited, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#>") {
			inComments = false
			currentParent = discussion
			continue
		}

		if replyb.Len() == 0 && line == "" {
			continue
		}

		if strings.HasPrefix(line, "#>") {
			quoted := strings.TrimSpace(strings.TrimPrefix(line, "#>"))
			if quoted == "comments:" {
				inComments = true
				currentParent = discussion
				continue
			}

			// keep track of which comment the reply body shows up below of
			// so we can assign it as a reply to that specifically
			fields := strings.Fields(quoted)
			if inComments && len(fields) > 0 && fields[0][0] == '[' && fields[0][len(fields[0])-1] == ']' {
				currId := fields[0][1 : len(fields[0])-1]
				for _, comment := range comments {
					if strings.HasPrefix(comment.ID.Hex(), currId) {
						currentParent = comment
						break
					}
				}
			}

			continue
		}

		// if we reach here this is a line for the reply input from the user
		replyb.WriteString(line)
		replyb.WriteByte('\n')

		if line == "" {
			continue
		}

		if selectedParent != nostr.ZeroID && selectedParent != currentParent.ID {
			return "", nostr.RelayEvent{}, fmt.Errorf("can only reply to one comment or create a top-level comment, got replies to both %s and %s", selectedParent.Hex()[0:6], currentParent.ID.Hex()[0:6])
		}

		selectedParent = currentParent.ID
	}

	content := strings.TrimSpace(replyb.String())
	if content == "" {
		return "", nostr.RelayEvent{}, fmt.Errorf("empty reply content, aborting")
	}

	if selectedParent == nostr.ZeroID || selectedParent == discussion.ID {
		return content, discussion, nil
	}

	for _, comment := range comments {
		if comment.ID == selectedParent {
			return content, comment, nil
		}
	}

	panic("selected reply parent not found (this never happens)")
}

func threadReplyEditorTemplate(ctx context.Context, headerLines []string, discussion nostr.RelayEvent, comments []nostr.RelayEvent) string {
	lines := make([]string, 0, len(headerLines)+3)
	for _, line := range headerLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, "# "+strings.TrimSpace(line))
	}
	lines = append(lines,
		"# write your reply here.",
		"# lines starting with '#' are ignored.",
		"",
	)

	appender := &lineAppender{lines, "#> "}

	printThreadMetadata(ctx, appender, discussion, "", false)

	for _, line := range strings.Split(discussion.Content, "\n") {
		appender.lines = append(appender.lines, "#> "+line)
	}

	if len(comments) > 0 {
		appender.lines = append(appender.lines, "#> ", "#> comments:")
		printThreadedComments(ctx, appender, comments, discussion.ID, false)
		appender.lines = append(appender.lines, "", "# comment below an existing comment to send yours as a reply to it.")
	}

	return strings.Join(appender.lines, "\n")
}

func keyerIdentity(ctx context.Context, kr nostr.Keyer) (nostr.PubKey, string, string, error) {
	pk, err := kr.GetPublicKey(ctx)
	if err != nil {
		return nostr.ZeroPK, "", "", err
	}

	meta := sys.FetchProfileMetadata(ctx, pk)
	return pk, meta.ShortName(), meta.NpubShort(), nil
}

func authorPreview(ctx context.Context, pubkey nostr.PubKey) string {
	meta := sys.FetchProfileMetadata(ctx, pubkey)
	if meta.Name != "" {
		return meta.ShortName() + " (" + meta.NpubShort() + ")"
	}
	return meta.NpubShort()
}

type lineAppender struct {
	lines  []string
	prefix string
}

func (l *lineAppender) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		line = strings.TrimRight(line, " ")
		l.lines = append(l.lines, l.prefix+line)
	}

	return len(b), nil
}
