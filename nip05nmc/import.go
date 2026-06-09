package nip05nmc

import (
	"encoding/json"
	"strings"
)

// DefaultMaxImportDepth is the minimum recursion depth that ifa-0001
// §"import" requires implementations to support. We default to four;
// callers may pass a higher budget but must not pass less without
// understanding the consequences (chains beyond this point are
// silently truncated).
const DefaultMaxImportDepth = 4

// nameValueLookup is the synchronous lookup callback used by
// expandImports. It returns the raw JSON value string for a Namecoin
// name, or the empty string if the name does not exist / could not
// be fetched / yielded malformed JSON. The contract is lenient: a
// failed lookup is treated as if the imported value were the empty
// object {} so transient ElectrumX hiccups do not nuke an otherwise
// resolvable record.
//
// The callback is invoked at most once per (name, selector) pair
// within a single top-level expansion.
type nameValueLookup func(namecoinName string) string

// expandImports walks the ifa-0001 §"import" chain rooted at root
// and returns a single merged object with no "import" key. The
// importing object's items always win; null values in the importer
// suppress the corresponding imported value (semantic deletion).
//
// Behaviour rules per ifa-0001 §"import":
//
//  1. No "import" key   → return root unchanged (zero extra I/O).
//  2. Four shorthand forms for the import value, all canonicalised
//     to [][2]string{{name, selector}, ...}:
//     • "d/foo"                  → [["d/foo", ""]]
//     • ["d/foo"]                → [["d/foo", ""]]
//     • ["d/foo", "selector"]    → [["d/foo", "selector"]]
//     • [["d/foo", "sel"], ...]  → canonical, as-is
//     Malformed import values (number, bool, object, wrong shape)
//     are silently dropped; the importer's own fields still apply.
//  3. Selector walk on the imported value follows ifa-0001 §"map":
//     exact label > "*" wildcard > "" default, walked right-to-left
//     (DNS-rightmost label first).
//  4. Merge is recursive with importer-wins semantics. JSON null in
//     the importer suppresses the imported value for that key.
//     Object values merge recursively; non-object values replace
//     wholesale.
//  5. Recursion budget defaults to DefaultMaxImportDepth (4). When
//     exhausted, the partial merge still applies — the importer's
//     own fields are not lost.
//  6. Cycle protection: visited (name|selector) pairs are tracked
//     within one top-level call.
//  7. Lenient I/O: lookup returning "" (or malformed JSON) → {}.
//  8. The "import" key is stripped from the returned object.
//
// The root is treated copy-on-write: callers may continue to use
// their original map after this returns.
func expandImports(root map[string]any, lookup nameValueLookup, maxDepth int) map[string]any {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxImportDepth
	}
	return expandRecursive(root, lookup, maxDepth, map[string]struct{}{})
}

func expandRecursive(obj map[string]any, lookup nameValueLookup, budget int, visited map[string]struct{}) map[string]any {
	rawImport, has := obj["import"]
	if !has {
		return obj
	}
	ops, ok := parseImportItem(rawImport)
	if !ok {
		// Malformed import: drop the key, keep the rest.
		return removeImportKey(obj)
	}
	if len(ops) == 0 || budget <= 0 {
		return removeImportKey(obj)
	}

	// Walk imports left-to-right. The spec is silent on multi-import
	// precedence; we follow the reference's convention that later
	// imports override earlier ones in the same array. The whole
	// accumulator still loses to the importing object on top.
	accumulator := map[string]any{}
	for _, op := range ops {
		key := op.name + "|" + op.selector
		if _, seen := visited[key]; seen {
			continue
		}
		visited[key] = struct{}{}
		func() {
			defer delete(visited, key)
			rawValue := lookup(op.name)
			if rawValue == "" {
				return
			}
			importedRoot, ok := tryParseObject(rawValue)
			if !ok {
				return
			}
			selectorView, ok := applySelector(importedRoot, op.selector)
			if !ok {
				return
			}
			expanded := expandRecursive(selectorView, lookup, budget-1, visited)
			accumulator = mergeImporterWins(expanded, accumulator)
		}()
	}

	// Finally merge the importing object (sans "import") on top.
	withoutImport := removeImportKey(obj)
	return mergeImporterWins(withoutImport, accumulator)
}

// mergeImporterWins returns a fresh map where every key in importer
// stays as-is (including JSON null, which acts as semantic delete on
// the imported counterpart) and keys present only in imported are
// added. Nested objects are merged recursively with the same rules.
func mergeImporterWins(importer, imported map[string]any) map[string]any {
	if len(imported) == 0 {
		// Still return a shallow copy so callers can mutate freely.
		out := make(map[string]any, len(importer))
		for k, v := range importer {
			out[k] = v
		}
		return out
	}
	if len(importer) == 0 {
		out := make(map[string]any, len(imported))
		for k, v := range imported {
			out[k] = v
		}
		return out
	}
	out := make(map[string]any, len(importer)+len(imported))
	for k, v := range imported {
		out[k] = v
	}
	for k, v := range importer {
		// If both sides are objects, recurse — except when the
		// importer's value is JSON null, which is a suppression
		// marker and must survive unchanged.
		if v == nil {
			out[k] = nil
			continue
		}
		if impObj, ok := v.(map[string]any); ok {
			if otherObj, ok := out[k].(map[string]any); ok {
				out[k] = mergeImporterWins(impObj, otherObj)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// applySelector walks the imported value's `map` tree to the node
// addressed by selector (DNS-dotted, e.g. "relay" or "a.b.c"). An
// empty selector or absent `map` returns root unchanged.
//
// Resolution rules per ifa-0001 §"map":
//   - Exact label match wins.
//   - "*" wildcard matches any single label.
//   - "" empty key is the default when no other match applies.
//   - A non-object child terminates the walk with (nil, false).
//
// The selector is read right-to-left: the rightmost label is the
// immediate child of the parent's `map` tree, matching DNS ordering.
func applySelector(root map[string]any, selector string) (map[string]any, bool) {
	if selector == "" {
		return root, true
	}
	labels := splitNonEmpty(selector, '.')
	if len(labels) == 0 {
		return root, true
	}
	// Reverse in place (DNS rightmost-first).
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}

	current := root
	for _, label := range labels {
		m, ok := current["map"].(map[string]any)
		if !ok {
			return nil, false
		}
		var child map[string]any
		if c, ok := m[label].(map[string]any); ok {
			child = c
		} else if c, ok := m["*"].(map[string]any); ok {
			child = c
		} else if c, ok := m[""].(map[string]any); ok {
			child = c
		} else {
			return nil, false
		}
		current = child
	}
	return current, true
}

func splitNonEmpty(s string, sep byte) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, string(sep))
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func tryParseObject(rawJSON string) (map[string]any, bool) {
	if rawJSON == "" {
		return nil, false
	}
	var dec any
	if err := json.Unmarshal([]byte(rawJSON), &dec); err != nil {
		return nil, false
	}
	obj, ok := dec.(map[string]any)
	if !ok {
		return nil, false
	}
	return obj, true
}

func removeImportKey(obj map[string]any) map[string]any {
	if _, has := obj["import"]; !has {
		out := make(map[string]any, len(obj))
		for k, v := range obj {
			out[k] = v
		}
		return out
	}
	out := make(map[string]any, len(obj)-1)
	for k, v := range obj {
		if k == "import" {
			continue
		}
		out[k] = v
	}
	return out
}

// importOp is one parsed entry from an `import` directive.
type importOp struct {
	name     string
	selector string // DNS-dotted, possibly empty.
}

// parseImportItem converts the `import` value into a flat list of
// importOps. The bool return is false only when the value is so
// malformed that no operations can be salvaged at all; in that case
// the caller drops the `import` key and keeps the importer's own
// fields. An empty (but well-formed) list returns ([], true).
//
// Accepted shapes:
//   - canonical:        [["d/foo"], ["d/bar","sub"]]
//   - bare string:      "d/foo"                       → [{d/foo, ""}]
//   - single-arr:       ["d/foo"]                     → [{d/foo, ""}]
//   - pair-arr:         ["d/foo","sub"]               → [{d/foo, "sub"}]
//
// Anything else (number, bool, object, mixed arrays) is malformed.
func parseImportItem(item any) ([]importOp, bool) {
	switch v := item.(type) {
	case string:
		name := strings.TrimSpace(v)
		if name == "" {
			return nil, false
		}
		return []importOp{{name: name, selector: ""}}, true
	case []any:
		if len(v) == 0 {
			return nil, true
		}
		// Canonical: first element is itself an array → array-of-arrays.
		if _, firstIsArr := v[0].([]any); firstIsArr {
			ops := make([]importOp, 0, len(v))
			for _, entry := range v {
				inner, ok := entry.([]any)
				if !ok {
					continue
				}
				op, ok := opFromArray(inner)
				if !ok {
					continue
				}
				ops = append(ops, op)
			}
			return ops, true
		}
		// Shorthand: ["name"] or ["name","selector"].
		op, ok := opFromArray(v)
		if !ok {
			return nil, false
		}
		return []importOp{op}, true
	default:
		return nil, false
	}
}

func opFromArray(arr []any) (importOp, bool) {
	if len(arr) == 0 {
		return importOp{}, false
	}
	name, ok := arr[0].(string)
	if !ok {
		return importOp{}, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return importOp{}, false
	}
	selector := ""
	if len(arr) >= 2 {
		s, ok := arr[1].(string)
		if !ok {
			return importOp{}, false
		}
		selector = strings.TrimSpace(s)
	}
	// Trailing dot is forbidden by spec; treat as malformed → no selector.
	if strings.HasSuffix(selector, ".") {
		return importOp{}, false
	}
	return importOp{name: name, selector: selector}, true
}
