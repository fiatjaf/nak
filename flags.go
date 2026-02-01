package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"fiatjaf.com/nostr"
	"github.com/markusmobius/go-dateparser"
	"github.com/urfave/cli/v3"
)

type (
	BoolIntFlag = cli.FlagBase[int, struct{}, boolIntValue]
)

type boolIntValue struct {
	int            int
	defaultWhenSet int
	hasDefault     bool
	hasBeenSet     bool
}

var _ cli.ValueCreator[int, struct{}] = boolIntValue{}

func (t boolIntValue) Create(val int, p *int, c struct{}) cli.Value {
	*p = val

	return &boolIntValue{
		defaultWhenSet: val,
		hasDefault:     true,
	}
}

func (t boolIntValue) IsBoolFlag() bool {
	return true
}

func (t boolIntValue) ToString(b int) string { return "<<>>" }

func (t *boolIntValue) Set(value string) error {
	t.hasBeenSet = true
	if value == "true" {
		if t.hasDefault {
			t.int = t.defaultWhenSet
		} else {
			t.int = 1
		}
		return nil
	} else {
		var err error
		t.int, err = strconv.Atoi(value)
		return err
	}
}

func (t *boolIntValue) String() string { return fmt.Sprintf("%#v", t.int) }
func (t *boolIntValue) Value() int     { return t.int }
func (t *boolIntValue) Get() any       { return t.int }

func getBoolInt(cmd *cli.Command, name string) int {
	return cmd.Value(name).(int)
}

//
//
//

type NaturalTimeFlag = cli.FlagBase[nostr.Timestamp, struct{}, naturalTimeValue]

type naturalTimeValue struct {
	timestamp  *nostr.Timestamp
	hasBeenSet bool
}

var _ cli.ValueCreator[nostr.Timestamp, struct{}] = naturalTimeValue{}

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
		if err != nil {
			return err
		}
		ts = date.Time
	}

	if t.timestamp != nil {
		*t.timestamp = nostr.Timestamp(ts.Unix())
	}

	t.hasBeenSet = true
	return nil
}

func (t *naturalTimeValue) String() string          { return fmt.Sprintf("%#v", t.timestamp) }
func (t *naturalTimeValue) Value() *nostr.Timestamp { return t.timestamp }
func (t *naturalTimeValue) Get() any                { return *t.timestamp }

func getNaturalDate(cmd *cli.Command, name string) nostr.Timestamp {
	return cmd.Value(name).(nostr.Timestamp)
}

//
//
//

type (
	PubKeyFlag = cli.FlagBase[nostr.PubKey, struct{}, pubkeyValue]
)

type pubkeyValue struct {
	pubkey     nostr.PubKey
	hasBeenSet bool
}

var _ cli.ValueCreator[nostr.PubKey, struct{}] = pubkeyValue{}

func (t pubkeyValue) Create(val nostr.PubKey, p *nostr.PubKey, c struct{}) cli.Value {
	*p = val
	return &pubkeyValue{
		pubkey: val,
	}
}

func (t pubkeyValue) ToString(b nostr.PubKey) string { return t.pubkey.String() }

func (t *pubkeyValue) Set(value string) error {
	pubkey, err := parsePubKey(value)
	t.pubkey = pubkey
	t.hasBeenSet = true
	return err
}

func (t *pubkeyValue) String() string      { return fmt.Sprintf("%#v", t.pubkey) }
func (t *pubkeyValue) Value() nostr.PubKey { return t.pubkey }
func (t *pubkeyValue) Get() any            { return t.pubkey }

func getPubKey(cmd *cli.Command, name string) nostr.PubKey {
	return cmd.Value(name).(nostr.PubKey)
}

//
//
//

type (
	pubkeySlice     = cli.SliceBase[nostr.PubKey, struct{}, pubkeyValue]
	PubKeySliceFlag = cli.FlagBase[[]nostr.PubKey, struct{}, pubkeySlice]
)

func getPubKeySlice(cmd *cli.Command, name string) []nostr.PubKey {
	return cmd.Value(name).([]nostr.PubKey)
}

//
//
//

type (
	IDFlag = cli.FlagBase[nostr.ID, struct{}, idValue]
)

type idValue struct {
	id         nostr.ID
	hasBeenSet bool
}

var _ cli.ValueCreator[nostr.ID, struct{}] = idValue{}

func (t idValue) Create(val nostr.ID, p *nostr.ID, c struct{}) cli.Value {
	*p = val
	return &idValue{
		id: val,
	}
}
func (t idValue) ToString(b nostr.ID) string { return t.id.String() }

func (t *idValue) Set(value string) error {
	id, err := parseEventID(value)
	t.id = id
	t.hasBeenSet = true
	return err
}

func (t *idValue) String() string  { return fmt.Sprintf("%#v", t.id) }
func (t *idValue) Value() nostr.ID { return t.id }
func (t *idValue) Get() any        { return t.id }

func getID(cmd *cli.Command, name string) nostr.ID {
	return cmd.Value(name).(nostr.ID)
}

//
//
//

type (
	idSlice     = cli.SliceBase[nostr.ID, struct{}, idValue]
	IDSliceFlag = cli.FlagBase[[]nostr.ID, struct{}, idSlice]
)

func getIDSlice(cmd *cli.Command, name string) []nostr.ID {
	return cmd.Value(name).([]nostr.ID)
}
