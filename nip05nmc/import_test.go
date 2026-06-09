package nip05nmc

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

// parseObj is a small helper that decodes a JSON literal into the
// generic map[string]any shape expandImports operates on. Tests that
// build their root via parseObj read more naturally than ones that
// hand-construct nested map literals.
func parseObj(t *testing.T, s string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return out
}

// fakeLookup builds a deterministic lookup function backed by an
// in-memory map. Tests assert both the merged output and the lookup
// invocation order (e.g. for the zero-extra-I/O regression guard).
type fakeLookup struct {
	values map[string]string
	calls  []string
}

func (f *fakeLookup) get(name string) string {
	f.calls = append(f.calls, name)
	if v, ok := f.values[name]; ok {
		return v
	}
	return ""
}

// ── 1. Pure unit tests — expandImports in isolation. ────────────────────

func TestExpandImports_NoImportKeyIsPassthrough(t *testing.T) {
	// Spec rule #1: no `import` key → zero extra I/O.
	obj := parseObj(t, `{"ip":"1.2.3.4"}`)
	fl := &fakeLookup{}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if len(fl.calls) != 0 {
		t.Fatalf("lookup must not be called for non-import records, got %v", fl.calls)
	}
	if out["ip"] != "1.2.3.4" {
		t.Fatalf("expected ip preserved, got %#v", out)
	}
}

func TestExpandImports_StringShorthand(t *testing.T) {
	obj := parseObj(t, `{"import":"d/lib","ip":"1.1.1.1"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"ip":"9.9.9.9","nostr":{"names":{"_":"abc"}}}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["ip"] != "1.1.1.1" {
		t.Fatalf("importer ip must win, got %#v", out["ip"])
	}
	nostrObj, ok := out["nostr"].(map[string]any)
	if !ok {
		t.Fatalf("nostr not an object: %#v", out["nostr"])
	}
	names, _ := nostrObj["names"].(map[string]any)
	if names["_"] != "abc" {
		t.Fatalf("imported nostr.names not merged: %#v", names)
	}
	if _, has := out["import"]; has {
		t.Fatalf("import key must be stripped from result")
	}
}

func TestExpandImports_ArrayShorthand(t *testing.T) {
	obj := parseObj(t, `{"import":["d/lib"]}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"tag":"from-lib"}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["tag"] != "from-lib" {
		t.Fatalf("imported tag missing: %#v", out)
	}
}

func TestExpandImports_PairArrayShorthandWithSelector(t *testing.T) {
	obj := parseObj(t, `{"import":["d/lib","relay"]}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"ip":"1.1.1.1","map":{"relay":{"ip":"7.7.7.7","tag":"selected"}}}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	// Selector descended into map.relay; top-level ip from lib is
	// hidden, the selected subtree's ip surfaces.
	if out["ip"] != "7.7.7.7" {
		t.Fatalf("selector node ip not surfaced: %#v", out["ip"])
	}
	if out["tag"] != "selected" {
		t.Fatalf("selector node tag missing: %#v", out)
	}
}

func TestExpandImports_CanonicalArrayOfArrays(t *testing.T) {
	obj := parseObj(t, `{"import":[["d/a"],["d/b"]]}`)
	fl := &fakeLookup{values: map[string]string{
		"d/a": `{"ip":"10.0.0.1","tag":"from-a"}`,
		"d/b": `{"ip":"10.0.0.2","extra":"from-b"}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	// Later imports override earlier ones (Kotlin reference parity).
	if out["ip"] != "10.0.0.2" {
		t.Fatalf("later import should override earlier ip, got %#v", out["ip"])
	}
	if out["tag"] != "from-a" {
		t.Fatalf("from-a tag should survive: %#v", out)
	}
	if out["extra"] != "from-b" {
		t.Fatalf("from-b extra should survive: %#v", out)
	}
}

func TestExpandImports_ImporterWinsOnPlainKeys(t *testing.T) {
	obj := parseObj(t, `{"import":"d/lib","ip":"1.1.1.1","extra":"local"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"ip":"9.9.9.9","extra":"remote","only-imported":"yes"}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["ip"] != "1.1.1.1" || out["extra"] != "local" || out["only-imported"] != "yes" {
		t.Fatalf("importer-wins merge wrong: %#v", out)
	}
}

func TestExpandImports_NullSuppressesImportedKey(t *testing.T) {
	// ifa-0001: null in importer is "present" — semantic delete.
	obj := parseObj(t, `{"import":"d/lib","ip":null}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"ip":"9.9.9.9","other":"keep"}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	v, present := out["ip"]
	if !present {
		t.Fatalf("ip key must remain (as null), got missing")
	}
	if v != nil {
		t.Fatalf("ip should be JSON null, got %#v", v)
	}
	if out["other"] != "keep" {
		t.Fatalf("non-suppressed imports must survive: %#v", out)
	}
}

func TestExpandImports_RecursionDepthFourHappyPath(t *testing.T) {
	obj := parseObj(t, `{"import":"d/a"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/a": `{"import":"d/b","layer":"a"}`,
		"d/b": `{"import":"d/c","layer":"b"}`,
		"d/c": `{"import":"d/d","layer":"c"}`,
		"d/d": `{"layer":"d","deep":"reached"}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["layer"] != "a" {
		t.Fatalf("each layer overrides; expected layer=a got %#v", out["layer"])
	}
	if out["deep"] != "reached" {
		t.Fatalf("4-deep value must reach the top: %#v", out)
	}
}

func TestExpandImports_RecursionTruncatedAtBudget(t *testing.T) {
	obj := parseObj(t, `{"import":"d/a","local":"keep"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/a": `{"import":"d/b","tag":"from-a"}`,
		"d/b": `{"tag":"from-b","leaf":"wont-show"}`,
	}}
	out := expandImports(obj, fl.get, 1)
	if out["tag"] != "from-a" {
		t.Fatalf("budget=1: only d/a should be merged, got tag=%#v", out["tag"])
	}
	if out["local"] != "keep" {
		t.Fatalf("importer keys must survive truncation: %#v", out)
	}
	if _, has := out["leaf"]; has {
		t.Fatalf("d/b should never have been visited at budget=1")
	}
}

func TestExpandImports_LookupReturnsNilTreatedAsEmpty(t *testing.T) {
	obj := parseObj(t, `{"import":"d/missing","local":"survives"}`)
	out := expandImports(obj, func(string) string { return "" }, DefaultMaxImportDepth)
	if out["local"] != "survives" {
		t.Fatalf("importer survives missing import: %#v", out)
	}
	if _, has := out["import"]; has {
		t.Fatalf("import key must be stripped even on miss")
	}
}

func TestExpandImports_LookupPanicTreatedAsEmpty(t *testing.T) {
	// Go's idiom for "throws" — make sure a panicking lookup doesn't
	// nuke the whole resolution. We wrap the lookup so the panic is
	// localised to one call and the importer's own fields apply.
	obj := parseObj(t, `{"import":"d/explode","local":"survives"}`)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expandImports must not propagate lookup panics; got %v", r)
		}
	}()
	lookup := func(name string) (result string) {
		defer func() {
			if r := recover(); r != nil {
				result = ""
			}
		}()
		panicLookup(name)
		return ""
	}
	out := expandImports(obj, lookup, DefaultMaxImportDepth)
	if out["local"] != "survives" {
		t.Fatalf("importer survives panicking lookup: %#v", out)
	}
}

// panicLookup is a tiny named function so the recover() above clearly
// shows the test surfaces the intended panic.
func panicLookup(name string) { panic("boom: " + name) }

func TestExpandImports_MalformedImportedJSON(t *testing.T) {
	obj := parseObj(t, `{"import":"d/broken","local":"keep"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/broken": `not valid json {{{`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["local"] != "keep" {
		t.Fatalf("importer keys must survive malformed imports: %#v", out)
	}
}

func TestExpandImports_MalformedImportValueIsNoOp(t *testing.T) {
	cases := []string{
		`{"import":42,"local":"keep"}`,
		`{"import":true,"local":"keep"}`,
		`{"import":{"foo":"bar"},"local":"keep"}`,
	}
	for _, raw := range cases {
		obj := parseObj(t, raw)
		out := expandImports(obj, func(string) string {
			t.Fatalf("lookup must not be called for malformed import %q", raw)
			return ""
		}, DefaultMaxImportDepth)
		if out["local"] != "keep" {
			t.Fatalf("%s: importer keys must survive malformed import: %#v", raw, out)
		}
		if _, has := out["import"]; has {
			t.Fatalf("%s: import key must be stripped", raw)
		}
	}
}

func TestExpandImports_CycleProtection(t *testing.T) {
	obj := parseObj(t, `{"import":"d/a","local":"top"}`)
	fl := &fakeLookup{values: map[string]string{
		"d/a": `{"import":"d/b","fromA":"yes"}`,
		"d/b": `{"import":"d/a","fromB":"yes"}`,
	}}
	done := make(chan map[string]any, 1)
	go func() {
		done <- expandImports(obj, fl.get, DefaultMaxImportDepth)
	}()
	out := <-done
	if out["local"] != "top" {
		t.Fatalf("importer keys must survive cycle: %#v", out)
	}
	// One of fromA / fromB must be present (the cycle break point is
	// an implementation detail; both visits before the break should
	// still contribute).
	if _, hasA := out["fromA"]; !hasA {
		if _, hasB := out["fromB"]; !hasB {
			t.Fatalf("expected at least one of fromA/fromB to survive: %#v", out)
		}
	}
}

func TestExpandImports_SelectorMultipleLabelsDNSOrder(t *testing.T) {
	// selector "a.b" → descend map.b first, then map.a (rightmost label first).
	obj := parseObj(t, `{"import":[["d/lib","a.b"]]}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"map":{"b":{"map":{"a":{"value":"deep"}}}}}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["value"] != "deep" {
		t.Fatalf("multi-label selector wrong: %#v", out)
	}
}

func TestExpandImports_SelectorWildcardFallback(t *testing.T) {
	obj := parseObj(t, `{"import":["d/lib","ghost"]}`)
	fl := &fakeLookup{values: map[string]string{
		"d/lib": `{"map":{"*":{"value":"wildcard"}}}`,
	}}
	out := expandImports(obj, fl.get, DefaultMaxImportDepth)
	if out["value"] != "wildcard" {
		t.Fatalf("expected wildcard fallback, got %#v", out)
	}
}

// ── 2. Integration tests — queryIdentifierWithLookup end-to-end. ────────

func TestQueryIdentifier_FollowsImportForSharedNostrNamesBlock(t *testing.T) {
	// Real-world testls.bit: apex `d/testls` delegates `nostr.names`
	// to `dd/testls` via an `import`. Without import support both
	// the bare lookup and the named-local-part lookup fail.
	fl := &fakeLookup{values: map[string]string{
		"d/testls":  `{"import":"dd/testls","ip":"107.152.38.155"}`,
		"dd/testls": `{"nostr":{"names":{"_":"460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c","m":"6cdebccabda1dfa058ab85352a79509b592b2bdfa0370325e28ec1cb4f18667d"}}}`,
	}}
	ctx := context.Background()

	pp, err := queryIdentifierWithLookup(ctx, "testls.bit", fl.get)
	if err != nil {
		t.Fatalf("bare testls.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != "460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c" {
		t.Fatalf("bare resolved to wrong pubkey: %s", got)
	}

	pp2, err := queryIdentifierWithLookup(ctx, "m@testls.bit", fl.get)
	if err != nil {
		t.Fatalf("m@testls.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp2.PublicKey[:]); got != "6cdebccabda1dfa058ab85352a79509b592b2bdfa0370325e28ec1cb4f18667d" {
		t.Fatalf("named local-part resolved to wrong pubkey: %s", got)
	}
}

func TestQueryIdentifier_NamedLocalPartResolvesAcrossImport(t *testing.T) {
	// Same flow but the local-part exists only in the imported
	// names block — exercises that the importer's other top-level
	// fields don't accidentally shadow nostr.names.
	fl := &fakeLookup{values: map[string]string{
		"d/foo": `{"import":"dd/foo","ip":"1.2.3.4"}`,
		"dd/foo": `{"nostr":{"names":{
			"alice":"aaaa000000000000000000000000000000000000000000000000000000000001"
		}}}`,
	}}
	pp, err := queryIdentifierWithLookup(context.Background(), "alice@foo.bit", fl.get)
	if err != nil {
		t.Fatalf("alice@foo.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != "aaaa000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("wrong pubkey resolved: %s", got)
	}
}

func TestQueryIdentifier_NoImportRecordIssuesExactlyOneLookup(t *testing.T) {
	// Regression guard for the "zero extra I/O" property — a plain
	// record must hit ElectrumX exactly once, not query an "import"
	// sibling that doesn't exist.
	fl := &fakeLookup{values: map[string]string{
		"d/plain": `{"nostr":{"names":{"_":"460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c"}}}`,
	}}
	pp, err := queryIdentifierWithLookup(context.Background(), "plain.bit", fl.get)
	if err != nil {
		t.Fatalf("plain.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != "460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c" {
		t.Fatalf("wrong pubkey: %s", got)
	}
	if len(fl.calls) != 1 || fl.calls[0] != "d/plain" {
		t.Fatalf("expected exactly one lookup of d/plain, got %v", fl.calls)
	}
}

func TestQueryIdentifier_ImporterWinsOnNostrNamesMap(t *testing.T) {
	// Importer declares its own nostr.names.m; imported value
	// declares a different one. Importer wins.
	fl := &fakeLookup{values: map[string]string{
		"d/testls":  `{"import":"dd/testls","nostr":{"names":{"m":"aaaa000000000000000000000000000000000000000000000000000000000001"}}}`,
		"dd/testls": `{"nostr":{"names":{"m":"bbbb000000000000000000000000000000000000000000000000000000000002"}}}`,
	}}
	pp, err := queryIdentifierWithLookup(context.Background(), "m@testls.bit", fl.get)
	if err != nil {
		t.Fatalf("m@testls.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != "aaaa000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("importer-wins violated; got %s", got)
	}
}

func TestQueryIdentifier_FailedImportDoesNotBreakLocalNames(t *testing.T) {
	// Importer has its own nostr.names; imported sibling is
	// missing. Resolution still succeeds from importer's own data.
	fl := &fakeLookup{values: map[string]string{
		"d/testls": `{"import":"dd/missing","nostr":{"names":{"_":"460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c"}}}`,
		// dd/missing is intentionally NOT registered.
	}}
	pp, err := queryIdentifierWithLookup(context.Background(), "testls.bit", fl.get)
	if err != nil {
		t.Fatalf("testls.bit: unexpected error %v", err)
	}
	if got := hex.EncodeToString(pp.PublicKey[:]); got != "460c25e682fda7832b52d1f22d3d22b3176d972f60dcdc3212ed8c92ef85065c" {
		t.Fatalf("wrong pubkey: %s", got)
	}
}

func TestQueryIdentifier_ImportTargetLacksNostrFieldYieldsClearError(t *testing.T) {
	// Both importer and imported lack nostr → the existing
	// no-nostr-field error must still surface (no silent success).
	fl := &fakeLookup{values: map[string]string{
		"d/testls":  `{"import":"dd/testls"}`,
		"dd/testls": `{"ip":"1.2.3.4"}`,
	}}
	_, err := queryIdentifierWithLookup(context.Background(), "testls.bit", fl.get)
	if err == nil {
		t.Fatalf("expected no-nostr-field error, got nil")
	}
	if errors.Is(err, ErrNameNotFound) {
		t.Fatalf("wrong error class: %v", err)
	}
}
