package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip11"
)

const nip66CheckTimeout = 7 * time.Second

type nip66Checks struct {
	open, read, write, nip11 bool
}

// parseNIP66Checks parses a comma-separated list of checks, like "open,read,nip11" or "all".
func parseNIP66Checks(spec string) (nip66Checks, error) {
	var checks nip66Checks
	for _, name := range strings.Split(spec, ",") {
		switch strings.TrimSpace(name) {
		case "open":
			checks.open = true
		case "read":
			checks.read = true
		case "write":
			checks.write = true
		case "nip11":
			checks.nip11 = true
		case "all":
			checks = nip66Checks{open: true, read: true, write: true, nip11: true}
		default:
			return checks, fmt.Errorf("unknown nip66 check '%s', must be one of open, read, write, nip11 or all", name)
		}
	}
	return checks, nil
}

// probeRelayNIP66 measures a relay and returns an unsigned kind:30166 relay discovery event.
// the relay is always connected to, but only the selected checks end up as tags.
func probeRelayNIP66(ctx context.Context, url string, checks nip66Checks) (nostr.Event, error) {
	url = nostr.NormalizeURL(url)
	if url == "" {
		return nostr.Event{}, fmt.Errorf("invalid relay url")
	}

	evt := nostr.Event{
		Kind:      30166,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"d", url}, {"n", nip66NetworkType(url)}},
	}

	requirements := map[string]bool{}

	// nip11 (a missing or broken document doesn't prevent the other checks)
	if checks.nip11 {
		nip11Ctx, cancel := context.WithTimeout(ctx, nip66CheckTimeout)
		info, err := nip11.Fetch(nip11Ctx, url)
		cancel()
		if err != nil {
			logverbose("nip11 check on %s failed: %s\n", url, err)
		} else {
			if j, err := json.Marshal(info); err == nil {
				evt.Content = string(j)
			}
			for _, n := range info.SupportedNIPs {
				switch v := n.(type) {
				case string:
					evt.Tags = append(evt.Tags, nostr.Tag{"N", v})
				case int:
					evt.Tags = append(evt.Tags, nostr.Tag{"N", strconv.Itoa(v)})
				}
			}
			if l := info.Limitation; l != nil {
				requirements["auth"] = l.AuthRequired
				requirements["payment"] = l.PaymentRequired
				requirements["writes"] = l.RestrictedWrites
				requirements["pow"] = l.MinPowDifficulty > 0
			}
			for _, t := range info.Tags {
				evt.Tags = append(evt.Tags, nostr.Tag{"t", t})
			}
		}
	}

	// open
	connectCtx, cancel := context.WithTimeout(ctx, nip66CheckTimeout)
	start := time.Now()
	relay, err := nostr.RelayConnect(connectCtx, url, sys.Pool.RelayOptions)
	cancel()
	if err != nil {
		return nostr.Event{}, fmt.Errorf("failed to connect: %w", err)
	}
	defer relay.Close()
	if checks.open {
		evt.Tags = append(evt.Tags, nostr.Tag{"rtt-open", nip66Millis(time.Since(start))})
	}

	// read
	readNeedsAuth := false
	if checks.read {
		if rtt, err := nip66MeasureRead(ctx, relay); err == nil {
			evt.Tags = append(evt.Tags, nostr.Tag{"rtt-read", nip66Millis(rtt)})
		} else {
			logverbose("read check on %s failed: %s\n", url, err)
			if strings.Contains(err.Error(), "auth-required:") {
				readNeedsAuth = true
				requirements["auth"] = true
			}
		}
	}

	// write (this publishes an event to the relay)
	if checks.write {
		rtt, err := nip66MeasureWrite(ctx, relay)
		if err == nil {
			evt.Tags = append(evt.Tags, nostr.Tag{"rtt-write", nip66Millis(rtt)})
			// what actually happened trumps what the nip11 document says
			requirements["auth"] = readNeedsAuth
			requirements["writes"] = false
		} else {
			logverbose("write check on %s failed: %s\n", url, err)
			msg := err.Error()
			if strings.Contains(msg, "auth-required:") {
				requirements["auth"] = true
			} else if strings.Contains(msg, "restricted:") || strings.Contains(msg, "blocked:") {
				requirements["writes"] = true
			}
		}
	}

	for _, key := range []string{"auth", "writes", "pow", "payment"} {
		value, ok := requirements[key]
		if !ok {
			continue
		}
		if !value {
			key = "!" + key
		}
		evt.Tags = append(evt.Tags, nostr.Tag{"R", key})
	}

	return evt, nil
}

func nip66MeasureRead(ctx context.Context, relay *nostr.Relay) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, nip66CheckTimeout)
	defer cancel()

	start := time.Now()
	sub, err := relay.Subscribe(ctx, nostr.Filter{Limit: 1}, nostr.SubscriptionOptions{
		Label: "nak-nip66",
		// don't let the library fake an EOSE, we want the real one
		MaxWaitForEOSE: time.Duration(math.MaxInt64),
	})
	if err != nil {
		return 0, err
	}
	defer sub.Unsub()

	for {
		select {
		case <-sub.Events:
			// the read round-trip ends at EOSE, ignore the events themselves
		case <-sub.EndOfStoredEvents:
			return time.Since(start), nil
		case reason := <-sub.ClosedReason:
			return 0, fmt.Errorf("subscription closed: %s", reason)
		case <-ctx.Done():
			return 0, fmt.Errorf("timed out waiting for EOSE")
		}
	}
}

func nip66MeasureWrite(ctx context.Context, relay *nostr.Relay) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, nip66CheckTimeout)
	defer cancel()

	// an ephemeral event signed by a throwaway key, so nothing gets stored
	evt := nostr.Event{
		Kind:      20166,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{},
		Content:   "nak nip66 write check",
	}
	if err := evt.Sign(nostr.Generate()); err != nil {
		return 0, err
	}

	start := time.Now()
	if err := relay.Publish(ctx, evt); err != nil {
		// relays like khatru answer "mute:" when nobody is listening for an ephemeral event,
		// that is still an accepted write
		if !strings.Contains(err.Error(), "mute:") {
			return 0, err
		}
	}
	return time.Since(start), nil
}

func nip66NetworkType(url string) string {
	host := url
	if _, rest, ok := strings.Cut(host, "://"); ok {
		host = rest
	}
	host, _, _ = strings.Cut(host, "/")
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	switch {
	case strings.HasSuffix(host, ".onion"):
		return "tor"
	case strings.HasSuffix(host, ".i2p"):
		return "i2p"
	case strings.HasSuffix(host, ".loki"):
		return "loki"
	default:
		return "clearnet"
	}
}

func nip66Millis(d time.Duration) string {
	return strconv.FormatInt(d.Milliseconds(), 10)
}
