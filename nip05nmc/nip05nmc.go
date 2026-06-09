package nip05nmc

import (
	"context"
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
	return queryIdentifierWithLookup(ctx, identifier, electrumxLookup(ctx))
}

// electrumxLookup returns a nameValueLookup that fetches each name
// via the default ElectrumX server pool. Failures collapse to the
// empty string per the lenient nameValueLookup contract.
func electrumxLookup(ctx context.Context) nameValueLookup {
	client := NewElectrumClient()
	return func(namecoinName string) string {
		result, err := client.NameShowWithFallback(ctx, namecoinName, DefaultElectrumXServers)
		if err != nil || result == nil {
			return ""
		}
		return result.Value
	}
}

// queryIdentifierWithLookup is the lookup-injected core of
// QueryIdentifier. It is unexported so test code in this package can
// stub the Namecoin transport without standing up a live ElectrumX.
//
// The flow is:
//
//  1. Parse the identifier into name + local-part.
//  2. Fetch the apex name's value via lookup.
//  3. Parse the value as a JSON object; expand any ifa-0001 §"import"
//     chain via expandImports so that records like testls.bit (which
//     keep their nostr.names block in a sibling dd/ name) resolve.
//  4. Pull the nostr pubkey + relays out of the merged object.
func queryIdentifierWithLookup(ctx context.Context, identifier string, lookup nameValueLookup) (*nostr.ProfilePointer, error) {
	parsed := parseIdentifier(identifier)
	if parsed == nil {
		return nil, fmt.Errorf("nip05nmc: not a Namecoin identifier: %q", identifier)
	}

	apex := lookup(parsed.namecoinName)
	if apex == "" {
		return nil, ErrNameNotFound
	}

	root, ok := tryParseObject(apex)
	if !ok {
		return nil, fmt.Errorf("nip05nmc: name value for %q is not a JSON object", parsed.namecoinName)
	}

	// ifa-0001 §"import": merge any imported siblings into the apex
	// object before extracting nostr fields. Records without an
	// `import` key incur zero extra I/O.
	merged := expandImports(root, lookup, DefaultMaxImportDepth)

	pubkeyHex, relays, err := extractNostrFromObject(merged, parsed)
	if err != nil {
		return nil, err
	}

	pk, err := nostr.PubKeyFromHex(pubkeyHex)
	if err != nil {
		return nil, fmt.Errorf("nip05nmc: invalid pubkey %q in name value: %w", pubkeyHex, err)
	}
	_ = ctx // context is consumed by the lookup; kept on the signature for API parity
	return &nostr.ProfilePointer{
		PublicKey: pk,
		Relays:    relays,
	}, nil
}

// extractNostrFromValue is a thin adapter that decodes a raw
// Namecoin name value JSON and delegates to extractNostrFromObject.
// It is retained so the long-standing extractor tests continue to
// drive the same behaviour after the import-chain refactor.
func extractNostrFromValue(valueJSON string, parsed *parsedIdentifier) (string, []string, error) {
	root, ok := tryParseObject(valueJSON)
	if !ok {
		return "", nil, fmt.Errorf("nip05nmc: name value is not valid JSON: %q", valueJSON)
	}
	return extractNostrFromObject(root, parsed)
}

// extractNostrFromObject pulls the nostr pubkey + relay list out of
// an already-parsed (and import-expanded) Namecoin name value.
// Supports both the simple `"nostr": "hex"` form and the extended
// `"nostr": { "names": {...}, "relays": {...} }` form used by Amethyst.
func extractNostrFromObject(root map[string]any, parsed *parsedIdentifier) (string, []string, error) {
	nostrRaw, ok := root["nostr"]
	if !ok {
		return "", nil, errors.New("nip05nmc: name value has no \"nostr\" field")
	}

	// Simple form: "nostr": "hex-pubkey"
	if asString, ok := nostrRaw.(string); ok {
		if parsed.isDomain && parsed.localPart != "_" {
			return "", nil, fmt.Errorf("nip05nmc: simple nostr field only supports root lookup, got local-part %q", parsed.localPart)
		}
		if !hexPubKeyRegex.MatchString(asString) {
			return "", nil, errors.New("nip05nmc: nostr field is not a 32-byte hex pubkey")
		}
		return strings.ToLower(asString), nil, nil
	}

	// Extended form: object with "names" and optional "relays".
	asObject, ok := nostrRaw.(map[string]any)
	if !ok {
		return "", nil, errors.New("nip05nmc: nostr field is neither string nor object")
	}

	if parsed.isDomain {
		return extractFromDomainNamesObject(asObject, parsed)
	}
	return extractFromIdentityObject(asObject, parsed)
}

func extractFromDomainNamesObject(obj map[string]any, parsed *parsedIdentifier) (string, []string, error) {
	namesRaw, ok := obj["names"]
	if !ok {
		return "", nil, errors.New("nip05nmc: extended nostr object lacks \"names\"")
	}
	names, ok := namesRaw.(map[string]any)
	if !ok {
		return "", nil, errors.New("nip05nmc: nostr.names is not an object")
	}

	// Match priority: exact local-part → "_" root → first entry (only
	// when the caller asked for root).
	var pickedKey, pickedPubkey string
	if v, ok := stringFrom(names, parsed.localPart); ok && hexPubKeyRegex.MatchString(v) {
		pickedKey, pickedPubkey = parsed.localPart, v
	} else if v, ok := stringFrom(names, "_"); ok && hexPubKeyRegex.MatchString(v) {
		pickedKey, pickedPubkey = "_", v
	} else if parsed.localPart == "_" {
		// First entry (map iteration order is non-deterministic, so
		// this is a weak fallback — we accept the first valid pubkey).
		for k, raw := range names {
			v, ok := raw.(string)
			if !ok {
				continue
			}
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

func extractFromIdentityObject(obj map[string]any, parsed *parsedIdentifier) (string, []string, error) {
	// Try "pubkey" field.
	if pk, ok := stringFrom(obj, "pubkey"); ok && hexPubKeyRegex.MatchString(pk) {
		var relays []string
		if r, ok := obj["relays"]; ok {
			relays = stringSliceFrom(r)
		}
		return strings.ToLower(pk), relays, nil
	}

	// Fall back to NIP-05-like "names" with "_" root.
	if raw, ok := obj["names"]; ok {
		if names, ok := raw.(map[string]any); ok {
			if v, ok := stringFrom(names, "_"); ok && hexPubKeyRegex.MatchString(v) {
				relays := extractRelays(obj, v)
				return strings.ToLower(v), relays, nil
			}
		}
	}

	_ = parsed
	return "", nil, errors.New("nip05nmc: id/ nostr object has no valid pubkey")
}

func extractRelays(obj map[string]any, pubkey string) []string {
	raw, ok := obj["relays"]
	if !ok {
		return nil
	}
	relayMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if v, ok := relayMap[strings.ToLower(pubkey)]; ok {
		return stringSliceFrom(v)
	}
	if v, ok := relayMap[pubkey]; ok {
		return stringSliceFrom(v)
	}
	return nil
}

// stringFrom reads a string-valued key from a generic map, returning
// ("", false) if the key is missing or the value is not a string.
func stringFrom(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// stringSliceFrom coerces a JSON-decoded []any into []string, dropping
// non-string entries. Returns nil for non-array inputs.
func stringSliceFrom(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, entry := range arr {
		if s, ok := entry.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
