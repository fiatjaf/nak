package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"
	"github.com/markusmobius/go-dateparser"
	"github.com/nbd-wtf/go-nostr"
)

type NaturalTimeFlag = cli.FlagBase[nostr.Timestamp, struct{}, naturalTimeValue]

// wrap to satisfy golang's flag interface.
type naturalTimeValue struct {
	timestamp  *nostr.Timestamp
	hasBeenSet bool
}

var _ cli.ValueCreator[nostr.Timestamp, struct{}] = naturalTimeValue{}

// Below functions are to satisfy the ValueCreator interface

func (t naturalTimeValue) Create(val nostr.Timestamp, p *nostr.Timestamp, c struct{}) cli.Value {
	*p = val
	return &naturalTimeValue{
		timestamp: p,
	}
}

func (t naturalTimeValue) ToString(b nostr.Timestamp) string {
	ts := b.Time()

	if ts.IsZero() {
		return ""
	}
	return fmt.Sprintf("%v", ts)
}

// Timestamp constructor(for internal testing only)
func newTimestamp(timestamp nostr.Timestamp) *naturalTimeValue {
	return &naturalTimeValue{timestamp: &timestamp}
}

// Below functions are to satisfy the flag.Value interface

// Parses the string value to timestamp
func (t *naturalTimeValue) Set(value string) error {
	var ts time.Time
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		// when the input is a raw number, treat it as an exact timestamp
		ts = time.Unix(n, 0)
	} else if errors.Is(err, strconv.ErrRange) {
		// this means a huge number, so we should fail
		return err
	} else {
		// otherwise try to parse it as a human date string in natural language
		date, err := dateparser.Parse(&dateparser.Configuration{
			DefaultTimezone: time.Local,
			CurrentTime:     time.Now(),
		}, value)
		ts = date.Time
		if err != nil {
			return err
		}
	}

	if t.timestamp != nil {
		*t.timestamp = nostr.Timestamp(ts.Unix())
	}

	t.hasBeenSet = true
	return nil
}

// String returns a readable representation of this value (for usage defaults)
func (t *naturalTimeValue) String() string {
	return fmt.Sprintf("%#v", t.timestamp)
}

// Value returns the timestamp value stored in the flag
func (t *naturalTimeValue) Value() *nostr.Timestamp {
	return t.timestamp
}

// Get returns the flag structure
func (t *naturalTimeValue) Get() any {
	return *t.timestamp
}

func getNaturalDate(cmd *cli.Command, name string) nostr.Timestamp {
	return cmd.Value(name).(nostr.Timestamp)
}
