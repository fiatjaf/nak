package main

import (
	"fmt"

	"fiatjaf.com/nostr"
	"github.com/itchyny/gojq"
)

const eventJQPrelude = `
def tags(tagName): .tags | map(select(.[0] == tagName));
def tag(tagName): tags(tagName) | .[0];
def value(tagName): tag(tagName)[1];
def has(tagName): (tags(tagName) | length) > 0;
def hasnt(tagName): (tags(tagName) | length) == 0;
def has_value(tagName; tagValue): tags(tagName) | map(select(.[1] == tagValue)) | length > 0;
`

type jqProcessor func(nostr.Event) (any, bool, error)

func jqPrepare(expr string) (jqProcessor, error) {
	if expr == "" {
		return nil, nil
	}

	query, err := gojq.Parse(eventJQPrelude + expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression: %w", err)
	}

	return func(evt nostr.Event) (any, bool, error) {
		input, err := toJQInput(evt)
		if err != nil {
			return nil, false, err
		}

		iter := code.Run(input)
		for {
			v, ok := iter.Next()
			if !ok {
				return v, false, nil
			}

			if err, ok := v.(error); ok {
				return v, false, err
			}

			if jqTruthy(v) {
				return v, true, nil
			}
		}
	}, nil
}

func toJQInput(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal jq input: %w", err)
	}

	var input any
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal jq input: %w", err)
	}

	return input, nil
}

func jqTruthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	default:
		return true
	}
}
