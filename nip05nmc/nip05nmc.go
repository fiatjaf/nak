package nip05nmc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"fiatjaf.com/nostr"
)

var hexPubKeyRegex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// IsDotBit reports whether an identifier should be routed to Namecoin
// resolution instead of DNS-based NIP-05. It matches:
//
//   - "<anything>.bit"
//   - "alice@<anything>.bit"
//   - "d/<name>"
//   - "id/<name>"
//
// It is intentionally cheap: callers use it as a front-door check in
// hot paths.
func IsDotBit(identifier string) bool {
	if identifier == "" {
		return false
	}
	norm := strings.ToLower(strings.TrimSpace(identifier))
	norm = strings.TrimPrefix(norm, "nostr:")
	if strings.HasPrefix(norm, "d/") || strings.HasPrefix(norm, "id/") {
		return true
	}
	return strings.HasSuffix(norm, ".bit")
}

// parsedIdentifier captures the Namecoin name we need to query and the
// local-part within its value.
type parsedIdentifier struct {
	namecoinName string // e.g. "d/example" or "id/alice"
	localPart    string // e.g. "alice", or "_" for the root
	isDomain     bool   // true for d/ names, false for id/ names
}

// parseIdentifier breaks a user-supplied identifier into the Namecoin
// name + local-part pair. Returns nil for anything that isn't a valid
// .bit / d/ / id/ identifier.
func parseIdentifier(raw string) *parsedIdentifier {
	input := strings.TrimSpace(raw)
	// Strip an optional NIP-21 "nostr:" URI prefix so callers can pass
	// through the nak-style `nostr:alice@example.bit` form directly.
	if len(input) >= 6 && strings.EqualFold(input[:6], "nostr:") {
		input = input[6:]
	}
	lower := strings.ToLower(input)

	// Explicit namespace references.
	if strings.HasPrefix(lower, "d/") {
		return &parsedIdentifier{namecoinName: lower, localPart: "_", isDomain: true}
	}
	if strings.HasPrefix(lower, "id/") {
		return &parsedIdentifier{namecoinName: lower, localPart: "_", isDomain: false}
	}

	// NIP-05 shape: user@domain.bit
	if strings.Contains(input, "@") && strings.HasSuffix(lower, ".bit") {
		parts := strings.SplitN(input, "@", 2)
		if len(parts) != 2 {
			return nil
		}
		local := strings.ToLower(parts[0])
		if local == "" {
			local = "_"
		}
		domain := strings.TrimSuffix(strings.ToLower(parts[1]), ".bit")
		if domain == "" {
			return nil
		}
		return &parsedIdentifier{
			namecoinName: "d/" + domain,
			localPart:    local,
			isDomain:     true,
		}
	}

	// Bare domain: example.bit
	if strings.HasSuffix(lower, ".bit") {
		domain := strings.TrimSuffix(lower, ".bit")
		if domain == "" {
			return nil
		}
		return &parsedIdentifier{
			namecoinName: "d/" + domain,
			localPart:    "_",
			isDomain:     true,
		}
	}

	return nil
}

// QueryIdentifier resolves a Namecoin `.bit` (or `d/` / `id/`)
// identifier into a nostr.ProfilePointer. The signature mirrors
// fiatjaf.com/nostr/nip05.QueryIdentifier so that callers can fall
// through from one to the other without reshaping their code.
//
// The context deadline is respected: we ask ElectrumX to honour the
// same timeout the caller set on the HTTP-based NIP-05 path.
func QueryIdentifier(ctx context.Context, identifier string) (*nostr.ProfilePointer, error) {
	parsed := parseIdentifier(identifier)
	if parsed == nil {
		return nil, fmt.Errorf("nip05nmc: not a Namecoin identifier: %q", identifier)
	}

	client := NewElectrumClient()
	result, err := client.NameShowWithFallback(ctx, parsed.namecoinName, DefaultElectrumXServers)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrNameNotFound
	}

	pubkeyHex, relays, err := extractNostrFromValue(result.Value, parsed)
	if err != nil {
		return nil, err
	}

	pk, err := nostr.PubKeyFromHex(pubkeyHex)
	if err != nil {
		return nil, fmt.Errorf("nip05nmc: invalid pubkey %q in name value: %w", pubkeyHex, err)
	}
	return &nostr.ProfilePointer{
		PublicKey: pk,
		Relays:    relays,
	}, nil
}

// extractNostrFromValue parses the Namecoin name value JSON and pulls
// the relevant nostr pubkey + relay list out of it. Supports both the
// simple `"nostr": "hex"` form and the extended
// `"nostr": { "names": {...}, "relays": {...} }` form used by Amethyst.
func extractNostrFromValue(valueJSON string, parsed *parsedIdentifier) (string, []string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(valueJSON), &root); err != nil {
		return "", nil, fmt.Errorf("nip05nmc: name value is not valid JSON: %w", err)
	}
	nostrRaw, ok := root["nostr"]
	if !ok {
		return "", nil, errors.New("nip05nmc: name value has no \"nostr\" field")
	}

	// Simple form: "nostr": "hex-pubkey"
	var asString string
	if err := json.Unmarshal(nostrRaw, &asString); err == nil {
		if parsed.isDomain && parsed.localPart != "_" {
			return "", nil, fmt.Errorf("nip05nmc: simple nostr field only supports root lookup, got local-part %q", parsed.localPart)
		}
		if !hexPubKeyRegex.MatchString(asString) {
			return "", nil, errors.New("nip05nmc: nostr field is not a 32-byte hex pubkey")
		}
		return strings.ToLower(asString), nil, nil
	}

	// Extended form: object with "names" and optional "relays".
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(nostrRaw, &asObject); err != nil {
		return "", nil, fmt.Errorf("nip05nmc: nostr field is neither string nor object: %w", err)
	}

	if parsed.isDomain {
		return extractFromDomainNamesObject(asObject, parsed)
	}
	return extractFromIdentityObject(asObject, parsed)
}

func extractFromDomainNamesObject(obj map[string]json.RawMessage, parsed *parsedIdentifier) (string, []string, error) {
	namesRaw, ok := obj["names"]
	if !ok {
		return "", nil, errors.New("nip05nmc: extended nostr object lacks \"names\"")
	}
	var names map[string]string
	if err := json.Unmarshal(namesRaw, &names); err != nil {
		return "", nil, fmt.Errorf("nip05nmc: parse names map: %w", err)
	}

	// Match priority: exact local-part → "_" root → first entry (only
	// when the caller asked for root). Matches Kotlin reference.
	var pickedKey, pickedPubkey string
	if v, ok := names[parsed.localPart]; ok && hexPubKeyRegex.MatchString(v) {
		pickedKey, pickedPubkey = parsed.localPart, v
	} else if v, ok := names["_"]; ok && hexPubKeyRegex.MatchString(v) {
		pickedKey, pickedPubkey = "_", v
	} else if parsed.localPart == "_" {
		// First entry (map iteration order is non-deterministic, so
		// this is a weak fallback — we accept the first valid pubkey).
		for k, v := range names {
			if hexPubKeyRegex.MatchString(v) {
				pickedKey, pickedPubkey = k, v
				break
			}
		}
	}
	if pickedPubkey == "" {
		return "", nil, fmt.Errorf("nip05nmc: no valid pubkey for local-part %q", parsed.localPart)
	}

	relays := extractRelays(obj, pickedPubkey)
	_ = pickedKey // kept for potential future use (ProfilePointer has no name field)
	return strings.ToLower(pickedPubkey), relays, nil
}

func extractFromIdentityObject(obj map[string]json.RawMessage, parsed *parsedIdentifier) (string, []string, error) {
	// Try "pubkey" field.
	if raw, ok := obj["pubkey"]; ok {
		var pk string
		if err := json.Unmarshal(raw, &pk); err == nil && hexPubKeyRegex.MatchString(pk) {
			// Try "relays" array (id/ shape).
			var relays []string
			if r, ok := obj["relays"]; ok {
				_ = json.Unmarshal(r, &relays)
			}
			return strings.ToLower(pk), relays, nil
		}
	}

	// Fall back to NIP-05-like "names" with "_" root.
	if raw, ok := obj["names"]; ok {
		var names map[string]string
		if err := json.Unmarshal(raw, &names); err == nil {
			if v, ok := names["_"]; ok && hexPubKeyRegex.MatchString(v) {
				relays := extractRelays(obj, v)
				return strings.ToLower(v), relays, nil
			}
		}
	}

	return "", nil, errors.New("nip05nmc: id/ nostr object has no valid pubkey")
}

func extractRelays(obj map[string]json.RawMessage, pubkey string) []string {
	raw, ok := obj["relays"]
	if !ok {
		return nil
	}
	var relayMap map[string][]string
	if err := json.Unmarshal(raw, &relayMap); err != nil {
		return nil
	}
	if v, ok := relayMap[strings.ToLower(pubkey)]; ok {
		return v
	}
	if v, ok := relayMap[pubkey]; ok {
		return v
	}
	return nil
}
