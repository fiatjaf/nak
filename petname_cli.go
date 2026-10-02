package main

import (
	"context"
	"fmt"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip02"
	"fiatjaf.com/nostr/nip05"
	"fiatjaf.com/nostr/nip19"
	"github.com/urfave/cli/v3"
)

func parsePubKeyForCommand(ctx context.Context, c *cli.Command, value string) (nostr.PubKey, error) {
	value = strings.TrimPrefix(value, "nostr:")
	from := nostr.ZeroPK
	if strings.HasPrefix(value, "~") {
		parts, err := nip02.ParsePetnamePath(value)
		if err != nil {
			return nostr.ZeroPK, err
		}
		// Absolute roots must also work without access to a signing key.
		_, hexErr := nostr.PubKeyFromHex(parts[0])
		prefix, _, nip19Err := nip19.Decode(parts[0])
		absolute := hexErr == nil || nip05.IsValidIdentifier(parts[0]) ||
			(nip19Err == nil && (prefix == "npub" || prefix == "nprofile"))
		if !absolute {
			kr, _, err := gatherKeyerFromArguments(ctx, c)
			if err != nil {
				return nostr.ZeroPK, fmt.Errorf("failed to get petname identity: %w", err)
			}
			from, err = kr.GetPublicKey(ctx)
			if err != nil {
				return nostr.ZeroPK, fmt.Errorf("failed to get petname identity: %w", err)
			}
		}
	}
	return parsePubKey(value, from)
}

// Public-key flags need the fully parsed signer options, including --sec after
// the petname flag. Keep their raw input when the parser hands it over, then let
// the original value parser handle the resolved keys (and any address inputs)
// once every flag has been parsed and the signer is known.
type deferredPubkeyFlag[T any, C any, V cli.ValueCreator[T, C]] struct {
	*cli.FlagBase[T, C, V]
	pending []string
}

func (f *deferredPubkeyFlag[T, C, V]) PreParse() error {
	f.pending = nil
	return f.FlagBase.PreParse()
}

func (f *deferredPubkeyFlag[T, C, V]) Set(_ string, val string) error {
	f.pending = append(f.pending, val)
	return nil
}

func (f *deferredPubkeyFlag[T, C, V]) IsSet() bool {
	return len(f.pending) > 0 || f.FlagBase.IsSet()
}

func (f *deferredPubkeyFlag[T, C, V]) RunAction(ctx context.Context, c *cli.Command) error {
	pending := f.pending
	f.pending = nil
	for _, input := range pending {
		value := strings.TrimSpace(input)
		if strings.HasPrefix(strings.TrimPrefix(value, "nostr:"), "~") {
			pk, err := parsePubKeyForCommand(ctx, c, value)
			if err != nil {
				return fmt.Errorf("invalid value %q for --%s: %w", input, f.Name, err)
			}
			value = pk.Hex()
		}
		if err := f.FlagBase.Set(f.Name, value); err != nil {
			return fmt.Errorf("invalid value %q for --%s: %w", input, f.Name, err)
		}
	}
	return f.FlagBase.RunAction(ctx, c)
}

func deferPetnameFlag[T any, C any, V cli.ValueCreator[T, C]](f *cli.FlagBase[T, C, V]) cli.Flag {
	return &deferredPubkeyFlag[T, C, V]{FlagBase: f}
}

func deferPetnameFlags(c *cli.Command) {
	for i, flag := range c.Flags {
		switch flag := flag.(type) {
		case *PubKeyFlag:
			c.Flags[i] = deferPetnameFlag(flag)
		case *PubKeySliceFlag:
			c.Flags[i] = deferPetnameFlag(flag)
		case *PubKeyOrAddressFlag:
			c.Flags[i] = deferPetnameFlag(flag)
		}
	}
	for _, sub := range c.Commands {
		deferPetnameFlags(sub)
	}
}
