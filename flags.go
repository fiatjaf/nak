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
	pk, err := nostr.PubKeyFromHex(value)
	t.pubkey = pk
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
	pk, err := nostr.IDFromHex(value)
	t.id = pk
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
