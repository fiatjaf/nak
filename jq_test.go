package main

import (
	"reflect"
	"testing"

	"fiatjaf.com/nostr"
)

func jqTestEvents() []nostr.Event {
	sk := nostr.Generate()
	signed := nostr.Event{
		Kind:      1,
		CreatedAt: 1699485669,
		Tags:      nostr.Tags{{"e", "abc", "wss://relay.example"}, {"p", "def"}, {"t"}},
		Content:   "hello \"world\" \u3042 \n<tab>\t",
	}
	signed.Sign(sk)

	return []nostr.Event{
		{},
		{Kind: 7, CreatedAt: 1, Tags: nostr.Tags{}},
		signed,
	}
}

func TestToJQInputMatchesJSONRoundTrip(t *testing.T) {
	for _, evt := range jqTestEvents() {
		var expected any
		if err := json.Unmarshal([]byte(evt.String()), &expected); err != nil {
			t.Fatal(err)
		}

		got := any(toJQInput(evt))
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("mismatch for %s:\n got: %#v\nwant: %#v", evt, got, expected)
		}
	}
}

func BenchmarkJQProcessor(b *testing.B) {
	jq, err := jqPrepare(`select(.kind == 1) | .content`, true)
	if err != nil {
		b.Fatal(err)
	}
	evt := jqTestEvents()[2]

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := jq(evt); err != nil {
			b.Fatal(err)
		}
	}
}
