// Package nip05nmc resolves Namecoin `.bit` identifiers to Nostr pubkeys
// via the ElectrumX protocol.
//
// NIP-05 normally resolves "name@domain.tld" via an HTTPS lookup at
// https://domain.tld/.well-known/nostr.json. That design pins the
// identity to DNS, which is a centralised censorship surface.
//
// Namecoin is a decentralised name system, settled on its own blockchain.
// A `d/<domain>` entry in Namecoin can carry a "nostr" field containing
// either a single hex pubkey or a NIP-05-style `{names, relays}` object.
// This package parses user-supplied identifiers such as
// "alice@example.bit" or "example.bit", queries an ElectrumX server for
// the name's current value, extracts the `nostr` field, and returns a
// standard nostr.ProfilePointer.
//
// Ported, with thanks, from the Kotlin reference implementation in
// Amethyst (vitorpamplona/amethyst PR #1937) and Nostur (nostur-com PR #60).
//
// The public API intentionally mirrors fiatjaf.com/nostr/nip05 so it can
// be used as a drop-in fall-through:
//
//	if nip05nmc.IsDotBit(input) {
//	    pp, err := nip05nmc.QueryIdentifier(ctx, input)
//	    ...
//	} else if nip05.IsValidIdentifier(input) {
//	    pp, err := nip05.QueryIdentifier(ctx, input)
//	    ...
//	}
//
// No external dependencies beyond the Go standard library and the
// fiatjaf.com/nostr types that nak already uses.
package nip05nmc
