package clink

import "testing"

func TestNofferRoundTripInterop(t *testing.T) {
	encoded := "noffer1qszqqqqraqpszqqzp9hkven9wf0kzcnrqythwumn8ghj7un9d3shjtn90psk6urvv5hxxmmdqqs8rdct4njgvlyqhe4ss02ynsm5smen0yxlp6jthehfeeshhm35kes9yppf9"
	offer, err := DecodeNoffer(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Pubkey.Hex() != "71b70bace4867c80be6b083d449c37486f33790df0ea4bbe6e9ce617bee34b66" {
		t.Fatalf("pubkey %s", offer.Pubkey.Hex())
	}
	if offer.Relay != "wss://relay.example.com" {
		t.Fatalf("relay %s", offer.Relay)
	}
	if offer.Offer != "offer_abc" {
		t.Fatalf("offer %s", offer.Offer)
	}
	if offer.PriceType != 0 || !offer.HasPrice || offer.Price != 1000 {
		t.Fatalf("price fields %+v", offer)
	}
	again, err := EncodeNoffer(offer)
	if err != nil {
		t.Fatal(err)
	}
	round, err := DecodeNoffer(again)
	if err != nil {
		t.Fatal(err)
	}
	if round.Offer != offer.Offer || round.Relay != offer.Relay || round.Price != offer.Price {
		t.Fatalf("roundtrip mismatch %+v vs %+v", offer, round)
	}
}
