package main

import (
	"fmt"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
)

// referenceJsonLines is the previous getJsonsOrBlank logic, which tried a
// full parse of the accumulated buffer after every line.
func referenceJsonLines(lines []string) (results []string, failed bool) {
	var curr strings.Builder
	for _, line := range lines {
		curr.WriteString(line)
		var dummy any
		if err := json.Unmarshal([]byte(curr.String()), &dummy); err != nil {
			failed = true
			continue
		}
		failed = false
		results = append(results, curr.String())
		curr.Reset()
	}
	return results, failed
}

func accumulateJsonLines(lines []string) (results []string, failed bool) {
	var acc jsonLineAccumulator
	for _, line := range lines {
		if j, ok := acc.feed(line); ok {
			results = append(results, j)
		}
	}
	return results, acc.err() != nil
}

func eventLines(n int, pretty bool) []string {
	sk := nostr.Generate()
	var lines []string
	for i := range n {
		evt := nostr.Event{
			Kind:      1,
			CreatedAt: nostr.Timestamp(1699485669 + i),
			Tags:      nostr.Tags{{"e", "abc", "wss://relay.example"}, {"p", "def"}, {"t", "x"}},
			Content:   fmt.Sprintf("hello {world} [%d]", i),
		}
		evt.Sign(sk)
		if !pretty {
			lines = append(lines, evt.String())
			continue
		}
		var generic any
		json.Unmarshal([]byte(evt.String()), &generic)
		j, _ := json.MarshalIndent(generic, "", "  ")
		for line := range strings.SplitSeq(string(j), "\n") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func TestJsonLineAccumulatorMatchesReference(t *testing.T) {
	cases := [][]string{
		eventLines(3, true),
		eventLines(3, false),
		{`{"kind":1}`, `{"kind":2}`},
		{`{"kind":1}`, ``, `{"kind":2}`},
		{`{"kind":1}`, ``},
		{`{"a":`, `"}"}`},
		{`{"a":"}`, `"}`},
		{`[1,`, `2]`, `["]"`, `]`},
		{`{}{}`, `{}`},
		{`{} x`},
		{`{`, `}}`},
		{`"str"`, `12`, `null`, `true`},
		{`nak`, `{}`},
		{`{"a":1`},
		{``},
		{`]`, `[]`},
	}

	for _, lines := range cases {
		expected, expectedFailed := referenceJsonLines(lines)
		got, gotFailed := accumulateJsonLines(lines)
		if fmt.Sprint(got) != fmt.Sprint(expected) || gotFailed != expectedFailed {
			t.Errorf("mismatch for %q:\n got: %q (failed=%v)\nwant: %q (failed=%v)",
				lines, got, gotFailed, expected, expectedFailed)
		}
	}
}

func BenchmarkJsonLines(b *testing.B) {
	for _, bc := range []struct {
		name  string
		lines []string
	}{
		{"compact", eventLines(100, false)},
		{"pretty", eventLines(100, true)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if results, _ := accumulateJsonLines(bc.lines); len(results) != 100 {
					b.Fatalf("got %d results", len(results))
				}
			}
		})
	}
}
