package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/khatru"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// newTestRelay starts a local, in-process relay whose acceptance of events is
// controlled by accept. The websocket connection itself always succeeds; what
// varies is whether EVENTs are stored or rejected with an OK false.
func newTestRelay(t *testing.T, accept bool) string {
	t.Helper()

	relay := khatru.NewRelay()
	if accept {
		relay.OnEvent = func(ctx context.Context, event nostr.Event) (reject bool, msg string) {
			return false, ""
		}
		relay.StoreEvent = func(ctx context.Context, event nostr.Event) error { return nil }
	} else {
		relay.OnEvent = func(ctx context.Context, event nostr.Event) (reject bool, msg string) {
			return true, "restricted: this test relay rejects everything"
		}
	}

	server := httptest.NewServer(relay)
	t.Cleanup(server.Close)

	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func connectTestRelay(t *testing.T, url string) *nostr.Relay {
	t.Helper()

	r, err := nostr.RelayConnect(t.Context(), url, nostr.RelayOptions{})
	require.NoError(t, err, "test relay should always accept the websocket connection")
	t.Cleanup(func() { r.Close() })
	return r
}

func signedTestEvent(t *testing.T) nostr.Event {
	t.Helper()

	sec, err := nostr.SecretKeyFromHex("0000000000000000000000000000000000000000000000000000000000000b")
	require.NoError(t, err)

	evt := nostr.Event{
		Kind:      1,
		Content:   "hello",
		CreatedAt: nostr.Now(),
	}
	require.NoError(t, evt.Sign(sec))
	return evt
}

// TestPublishFlowFailsWhenAllRelaysRejectEvent is a regression test for
// https://github.com/fiatjaf/nak/issues/129 : if relays were given and every
// single one of them rejects the event (connection succeeds, publish fails),
// publishFlow must return a non-nil error so callers (and ultimately the
// process exit code) reflect the failure instead of silently reporting success.
func TestPublishFlowFailsWhenAllRelaysRejectEvent(t *testing.T) {
	relayURL := newTestRelay(t, false)
	relay := connectTestRelay(t, relayURL)
	evt := signedTestEvent(t)

	err := publishFlow(t.Context(), &cli.Command{}, nil, evt, []*nostr.Relay{relay})
	require.Error(t, err, "publishing to a relay that rejects the event should return an error")
	require.Contains(t, err.Error(), "failed to publish")
}

// TestPublishFlowSucceedsWhenARelayAcceptsEvent makes sure the fix for #129
// didn't turn a successful publish into a reported failure.
func TestPublishFlowSucceedsWhenARelayAcceptsEvent(t *testing.T) {
	relayURL := newTestRelay(t, true)
	relay := connectTestRelay(t, relayURL)
	evt := signedTestEvent(t)

	err := publishFlow(t.Context(), &cli.Command{}, nil, evt, []*nostr.Relay{relay})
	require.NoError(t, err)
}

// TestPublishFlowNoRelaysIsNotAnError makes sure that calling publishFlow with
// no relays at all (e.g. `nak event` with no relay arguments, just printing
// the event) still doesn't produce a spurious error.
func TestPublishFlowNoRelaysIsNotAnError(t *testing.T) {
	evt := signedTestEvent(t)

	err := publishFlow(t.Context(), &cli.Command{}, nil, evt, nil)
	require.NoError(t, err)
}
