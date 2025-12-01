package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip77"
	"fiatjaf.com/nostr/nip77/negentropy"
	"fiatjaf.com/nostr/nip77/negentropy/storage"
	"github.com/urfave/cli/v3"
)

var syncCmd = &cli.Command{
	Name:        "sync",
	Usage:       "sync events between two relays using negentropy",
	Description: `uses nip77 negentropy to sync events between two relays`,
	ArgsUsage:   "<relay1> <relay2>",
	Flags:       reqFilterFlags,
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args().Slice()
		if len(args) != 2 {
			return fmt.Errorf("need exactly two relay URLs: source and target")
		}

		filter := nostr.Filter{}
		if err := applyFlagsToFilter(c, &filter); err != nil {
			return err
		}

		peerA, err := NewRelayThirdPartyRemote(ctx, args[0])
		if err != nil {
			return fmt.Errorf("error setting up %s: %w", args[0], err)
		}

		peerB, err := NewRelayThirdPartyRemote(ctx, args[1])
		if err != nil {
			return fmt.Errorf("error setting up %s: %w", args[1], err)
		}

		tpn := NewThirdPartyNegentropy(
			peerA,
			peerB,
			filter,
		)

		wg := sync.WaitGroup{}

		wg.Go(func() {
			err = tpn.Run(ctx)
		})

		wg.Go(func() {
			type op struct {
				src *nostr.Relay
				dst *nostr.Relay
				ids []nostr.ID
			}

			pending := []op{
				{peerA.relay, peerB.relay, make([]nostr.ID, 0, 30)},
				{peerB.relay, peerA.relay, make([]nostr.ID, 0, 30)},
			}

			for delta := range tpn.Deltas {
				have := delta.Have.relay
				havenot := delta.HaveNot.relay
				logverbose("%s has %s, %s doesn't.\n", have.URL, delta.ID.Hex(), havenot.URL)

				idx := 0 // peerA
				if have == peerB.relay {
					idx = 1 // peerB
				}
				pending[idx].ids = append(pending[idx].ids, delta.ID)

				// every 30 ids do a fetch-and-publish
				if len(pending[idx].ids) == 30 {
					for evt := range pending[idx].src.QueryEvents(nostr.Filter{IDs: pending[idx].ids}) {
						pending[idx].dst.Publish(ctx, evt)
					}
					pending[idx].ids = pending[idx].ids[:0]
				}
			}

			// do it for the remaining ids
			for _, op := range pending {
				if len(op.ids) > 0 {
					for evt := range op.src.QueryEvents(nostr.Filter{IDs: op.ids}) {
						op.dst.Publish(ctx, evt)
					}
				}
			}
		})

		wg.Wait()

		return err
	},
}

type ThirdPartyNegentropy struct {
	PeerA  *RelayThirdPartyRemote
	PeerB  *RelayThirdPartyRemote
	Filter nostr.Filter

	Deltas chan Delta
}

type Delta struct {
	ID      nostr.ID
	Have    *RelayThirdPartyRemote
	HaveNot *RelayThirdPartyRemote
}

type boundKey string

func getBoundKey(b negentropy.Bound) boundKey {
	return boundKey(fmt.Sprintf("%d:%x", b.Timestamp, b.IDPrefix))
}

type RelayThirdPartyRemote struct {
	relay    *nostr.Relay
	messages chan string
	err      error
}

func NewRelayThirdPartyRemote(ctx context.Context, url string) (*RelayThirdPartyRemote, error) {
	rtpr := &RelayThirdPartyRemote{
		messages: make(chan string, 3),
	}

	var err error
	rtpr.relay, err = nostr.RelayConnect(ctx, url, nostr.RelayOptions{
		CustomHandler: func(data string) {
			envelope := nip77.ParseNegMessage(data)
			if envelope == nil {
				return
			}
			switch env := envelope.(type) {
			case *nip77.OpenEnvelope, *nip77.CloseEnvelope:
				rtpr.err = fmt.Errorf("unexpected %s received from relay", env.Label())
				return
			case *nip77.ErrorEnvelope:
				rtpr.err = fmt.Errorf("relay returned a %s: %s", env.Label(), env.Reason)
				return
			case *nip77.MessageEnvelope:
				rtpr.messages <- env.Message
			}
		},
	})
	if err != nil {
		return nil, err
	}

	return rtpr, nil
}

func (rtpr *RelayThirdPartyRemote) SendInitialMessage(filter nostr.Filter, msg string) error {
	msgj, _ := json.Marshal(nip77.OpenEnvelope{
		SubscriptionID: "sync3",
		Filter:         filter,
		Message:        msg,
	})
	return rtpr.relay.WriteWithError(msgj)
}

func (rtpr *RelayThirdPartyRemote) SendMessage(msg string) error {
	msgj, _ := json.Marshal(nip77.MessageEnvelope{
		SubscriptionID: "sync3",
		Message:        msg,
	})
	return rtpr.relay.WriteWithError(msgj)
}

func (rtpr *RelayThirdPartyRemote) SendClose() error {
	msgj, _ := json.Marshal(nip77.CloseEnvelope{
		SubscriptionID: "sync3",
	})
	return rtpr.relay.WriteWithError(msgj)
}

var thirdPartyRemoteEndOfMessages = errors.New("the-end")

func (rtpr *RelayThirdPartyRemote) Receive() (string, error) {
	if rtpr.err != nil {
		return "", rtpr.err
	}
	if msg, ok := <-rtpr.messages; ok {
		return msg, nil
	}
	return "", thirdPartyRemoteEndOfMessages
}

func NewThirdPartyNegentropy(peerA, peerB *RelayThirdPartyRemote, filter nostr.Filter) *ThirdPartyNegentropy {
	return &ThirdPartyNegentropy{
		PeerA:  peerA,
		PeerB:  peerB,
		Filter: filter,
		Deltas: make(chan Delta, 100),
	}
}

func (n *ThirdPartyNegentropy) Run(ctx context.Context) error {
	peerAIds := make(map[nostr.ID]struct{})
	peerBIds := make(map[nostr.ID]struct{})
	peerASkippedBounds := make(map[boundKey]struct{})
	peerBSkippedBounds := make(map[boundKey]struct{})

	// send an empty message to A to start things up
	initialMsg := createInitialMessage()
	err := n.PeerA.SendInitialMessage(n.Filter, initialMsg)
	if err != nil {
		return err
	}

	hasSentInitialMessageToB := false

	for {
		// receive message from A
		msgA, err := n.PeerA.Receive()
		if err != nil {
			return err
		}
		msgAb, _ := nostr.HexDecodeString(msgA)
		if len(msgAb) == 1 {
			break
		}

		msgToB, err := parseMessageBuildNext(
			msgA,
			peerBSkippedBounds,
			func(id nostr.ID) {
				if _, exists := peerBIds[id]; exists {
					delete(peerBIds, id)
				} else {
					peerAIds[id] = struct{}{}
				}
			},
			func(boundKey boundKey) {
				peerASkippedBounds[boundKey] = struct{}{}
			},
		)
		if err != nil {
			return err
		}

		// emit deltas from B after receiving message from A
		for id := range peerBIds {
			select {
			case n.Deltas <- Delta{ID: id, Have: n.PeerB, HaveNot: n.PeerA}:
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			delete(peerBIds, id)
		}

		if len(msgToB) == 2 {
			// exit condition (no more messages to send)
			break
		}

		// send message to B
		if hasSentInitialMessageToB {
			err = n.PeerB.SendMessage(msgToB)
		} else {
			err = n.PeerB.SendInitialMessage(n.Filter, msgToB)
			hasSentInitialMessageToB = true
		}
		if err != nil {
			return err
		}

		// receive message from B
		msgB, err := n.PeerB.Receive()
		if err != nil {
			return err
		}
		msgBb, _ := nostr.HexDecodeString(msgB)
		if len(msgBb) == 1 {
			break
		}

		msgToA, err := parseMessageBuildNext(
			msgB,
			peerASkippedBounds,
			func(id nostr.ID) {
				if _, exists := peerAIds[id]; exists {
					delete(peerAIds, id)
				} else {
					peerBIds[id] = struct{}{}
				}
			},
			func(boundKey boundKey) {
				peerBSkippedBounds[boundKey] = struct{}{}
			},
		)
		if err != nil {
			return err
		}

		// emit deltas from A after receiving message from B
		for id := range peerAIds {
			select {
			case n.Deltas <- Delta{ID: id, Have: n.PeerA, HaveNot: n.PeerB}:
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			delete(peerAIds, id)
		}

		if len(msgToA) == 2 {
			// exit condition (no more messages to send)
			break
		}

		// send message to A
		err = n.PeerA.SendMessage(msgToA)
		if err != nil {
			return err
		}
	}

	// emit remaining deltas before exit
	for id := range peerAIds {
		select {
		case n.Deltas <- Delta{ID: id, Have: n.PeerA, HaveNot: n.PeerB}:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	for id := range peerBIds {
		select {
		case n.Deltas <- Delta{ID: id, Have: n.PeerB, HaveNot: n.PeerA}:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}

	n.PeerA.SendClose()
	n.PeerB.SendClose()
	close(n.Deltas)

	return nil
}

func createInitialMessage() string {
	output := bytes.NewBuffer(make([]byte, 0, 64))
	output.WriteByte(negentropy.ProtocolVersion)

	dummy := negentropy.BoundWriter{}
	dummy.WriteBound(output, negentropy.InfiniteBound)
	output.WriteByte(byte(negentropy.FingerprintMode))

	// hardcoded random fingerprint
	fingerprint := [negentropy.FingerprintSize]byte{
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
	}
	output.Write(fingerprint[:])

	return nostr.HexEncodeToString(output.Bytes())
}

func parseMessageBuildNext(
	msg string,
	skippedBounds map[boundKey]struct{},
	idCallback func(id nostr.ID),
	skipCallback func(boundKey boundKey),
) (string, error) {
	msgb, err := nostr.HexDecodeString(msg)
	if err != nil {
		return "", err
	}

	br := &negentropy.BoundReader{}
	bw := &negentropy.BoundWriter{}

	nextMsg := bytes.NewBuffer(make([]byte, 0, len(msgb)))
	acc := &storage.Accumulator{} // this will be used for building our own fingerprints and also as a placeholder

	reader := bytes.NewReader(msgb)
	pv, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	if pv != negentropy.ProtocolVersion {
		return "", fmt.Errorf("unsupported protocol version %v", pv)
	}

	nextMsg.WriteByte(pv)

	for reader.Len() > 0 {
		bound, err := br.ReadBound(reader)
		if err != nil {
			return "", err
		}

		modeVal, err := negentropy.ReadVarInt(reader)
		if err != nil {
			return "", err
		}
		mode := negentropy.Mode(modeVal)

		switch mode {
		case negentropy.SkipMode:
			skipCallback(getBoundKey(bound))
			if _, skipped := skippedBounds[getBoundKey(bound)]; !skipped {
				bw.WriteBound(nextMsg, bound)
				negentropy.WriteVarInt(nextMsg, int(negentropy.SkipMode))
			}

		case negentropy.FingerprintMode:
			_, err = reader.Read(acc.Buf[0:negentropy.FingerprintSize] /* use this buffer as a dummy */)
			if err != nil {
				return "", err
			}

			if _, skipped := skippedBounds[getBoundKey(bound)]; !skipped {
				bw.WriteBound(nextMsg, bound)
				negentropy.WriteVarInt(nextMsg, int(negentropy.FingerprintMode))
				nextMsg.Write(acc.Buf[0:negentropy.FingerprintSize] /* idem */)
			}
		case negentropy.IdListMode:
			// when receiving an idlist we will never send this bound again to this peer
			skipCallback(getBoundKey(bound))

			// and instead of sending these ids to the other peer we'll send a fingerprint
			acc.Reset()

			numIds, err := negentropy.ReadVarInt(reader)
			if err != nil {
				return "", err
			}

			for range numIds {
				id := nostr.ID{}

				_, err = reader.Read(id[:])
				if err != nil {
					return "", err
				}

				idCallback(id)

				acc.AddBytes(id[:])
			}

			if _, skipped := skippedBounds[getBoundKey(bound)]; !skipped {
				fingerprint := acc.GetFingerprint(numIds)

				bw.WriteBound(nextMsg, bound)
				negentropy.WriteVarInt(nextMsg, int(negentropy.FingerprintMode))
				nextMsg.Write(fingerprint[:])
			}
		default:
			return "", fmt.Errorf("unknown mode %v", mode)
		}
	}

	return nostr.HexEncodeToString(nextMsg.Bytes()), nil
}
