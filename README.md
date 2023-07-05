# nostr army knife

this repository contains two things:

## a command-line tool for decoding and encoding nostr entities and talking to relays

Install with `go install github.com/fiatjaf/nak`.

It pairs nicely with https://github.com/blakejakopovic/nostcat using unix pipes.

### examples

```
~> nak decode nsec1aqc5q5l8da0l7u6gra4p5xhleclngezlpsgd7z5dx07cpu8sxf2shqgn6y
{
  "pubkey": "5b36b874b2b983197ba4be80553b2e4b6db2895a04567cea0aa47585b2e0c620",
  "private_key": "e8314053e76f5fff73481f6a1a1affce3f34645f0c10df0a8d33fd80f0f03255"
}

~> nak event -c hello --sec e8314053e76f5fff73481f6a1a1affce3f34645f0c10df0a8d33fd80f0f03255
{"id":"ed840ef37a40cce4f4b8c361e5df13457ad664209cf4a297fd7df7e84fdd32e0","pubkey":"5b36b874b2b983197ba4be80553b2e4b6db2895a04567cea0aa47585b2e0c620","created_at":1683201092,"kind":1,"tags":[],"content":"hello","sig":"304a87dbbdf986a187eb9417316cfe3d6f8f31793ba20c9c6d7e4ebeeefe950d6ecba6098c201b7170c04e27c2f920d607a90f5c8763c35ac806dce37df1d05d"}
~> nak event -c hello --sec e8314053e76f5fff73481f6a1a1affce3f34645f0c10df0a8d33fd80f0f03255 wss://relay.stoner.com wss://nos.lol wss://nostr.wine wss://atlas.nostr.land wss://relay.damus.io
{"id":"54a534647bdcd2751d743fea4fc9eee5dba613887d69425f0891d9c2f82772a5","pubkey":"5b36b874b2b983197ba4be80553b2e4b6db2895a04567cea0aa47585b2e0c620","created_at":1684895417,"kind":1,"tags":[],"content":"hello","sig":"81a14cfe628fab6cd6135bb66f6e8b3bb4bfce4f666462a1303fdfbc9038fd141e73db3fe7e774a8f023fc70622c50a67d4fa41d3d09806c78f051985c11e0bd"}
publishing to wss://relay.stoner.com... failed: msg: blocked: pubkey is not allowed to publish to this relay
publishing to wss://nos.lol... success.
publishing to wss://nostr.wine... failed: msg: blocked: not an active paid member
publishing to wss://atlas.nostr.land... failed: msg: blocked: pubkey not admitted
publishing to wss://relay.damus.io... success.

~> nak decode nevent1qqs29yet5tp0qq5xu5qgkeehkzqh5qu46739axzezcxpj4tjlkx9j7gpr4mhxue69uhkummnw3ez6ur4vgh8wetvd3hhyer9wghxuet5sh59ud
{
  "id": "a2932ba2c2f00286e5008b6737b0817a0395d7a25e9859160c195572fd8c5979",
  "relays": [
    "wss://nostr-pub.wellorder.net"
  ]
}

~> nak req -a a2932ba2c2f00286e5008b6737b0817a0395d7a25e9859160c195572fd8c5979 -k 1 -a e8b487c079b0f67c695ae6c4c2552a47f38adfa2533cc5926bd2c102942fdcb7
["REQ","nak",{"kinds":[1],"authors":["a2932ba2c2f00286e5008b6737b0817a0395d7a25e9859160c195572fd8c5979","e8b487c079b0f67c695ae6c4c2552a47f38adfa2533cc5926bd2c102942fdcb7"]}]

~> nak req -k 1 -l 1 --stream wss://relay.stoner.com
{"id":"1d73832917bf5a72276c53e9246c28b97225b51cd5735843434f7756fc0ddead","pubkey":"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d","created_at":1684894689,"kind":1,"tags":[["p","bcbeb5a2e6b547f6d0c3d8c16145f7bb94f3639ec7ecbcfe50045dbb2eede70b","wss://nos.lol","artk42"],["e","b5af6815c8d89a7d5b6201b9e624fbd5389fca3337ba2dc05c6187234a7c1bd5","wss://nos.lol","root"],["e","5795e27aff0a459a30c64a61a32c43d968cd19c8f1926cf01fc02e9da7c56f2b","wss://nos.lol","reply"],["client","coracle"]],"content":"Because that makes no sense.","sig":"3ee5b2b26ec6b116ef1a6b1c10bc7e56674a3c36841814f68b57f63259f3d78e23629d4599afe67e72c220e27b4b0966cc51adc1da808c8c6111dedb531ac0c3"}
```

### documentation

```
~> nak --help
NAME:
   nak - the nostr army knife command-line tool

USAGE:
   nak [global options] command [command options] [arguments...]

COMMANDS:
   req      generates encoded REQ messages and optionally use them to talk to relays
   event    generates an encoded event and either prints it or sends it to a set of relays
   decode   decodes nip19, nip21, nip05 or hex entities
   encode   encodes notes and other stuff to nip19 entities
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h  show help

~> nak event --help
NAME:
   nak event - generates an encoded event and either prints it or sends it to a set of relays

USAGE:
   nak event [command options] [arguments...]

DESCRIPTION:
   example usage (for sending directly to a relay with 'nostcat'):
       nak event -k 1 -c hello --envelope | nostcat wss://nos.lol
   standalone:
       nak event -k 1 -c hello wss://nos.lol`,

OPTIONS:
   --envelope   print the event enveloped in a ["EVENT", ...] message ready to be sent to a relay (default: false)
   --sec value  secret key to sign the event (default: the key '1')

   EVENT FIELDS

   --content value, -c value                        event content (default: hello from the nostr army knife)
   --created-at value, --time value, --ts value     unix timestamp value for the created_at field (default: now)
   --kind value, -k value                           event kind (default: 1)
   --tag value, -t value [ --tag value, -t value ]  sets a tag field on the event, takes a value like -t e=<id>
   -e value [ -e value ]                            shortcut for --tag e=<value>
   -p value [ -p value ]                            shortcut for --tag p=<value>

~> nak req --help
NAME:
   nak req - generates encoded REQ messages and optionally use them to talk to relays

USAGE:
   nak req [command options] [relay...]

DESCRIPTION:
   outputs a NIP-01 Nostr filter. when a relay is not given, will print the filter, otherwise will connect to the given relay and send the filter.

   example usage (with 'nostcat'):
       nak req -k 1 -a 3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d | nostcat wss://nos.lol
   standalone:
       nak req -k 1 wss://nos.lol

OPTIONS:
   --bare    when printing the filter, print just the filter, not enveloped in a ["REQ", ...] array (default: false)
   --stream  keep the subscription open, printing all events as they are returned (default: false, will close on EOSE)

   FILTER ATTRIBUTES

   --author value, -a value [ --author value, -a value ]  only accept events from these authors (pubkey as hex)
   --id value, -i value [ --id value, -i value ]          only accept events with these ids (hex)
   --kind value, -k value [ --kind value, -k value ]      only accept events with these kind numbers
   --limit value, -l value                                only accept up to this number of events (default: 0)
   --since value, -s value                                only accept events newer than this (unix timestamp) (default: 0)
   --tag value, -t value [ --tag value, -t value ]        takes a tag like -t e=<id>, only accept events with these tags
   --until value, -u value                                only accept events older than this (unix timestamp) (default: 0)
   -e value [ -e value ]                                  shortcut for --tag e=<value>
   -p value [ -p value ]                                  shortcut for --tag p=<value>


OPTIONS:
   --bare    when printing the filter, print just the filter, not enveloped in a ["REQ", ...] array (default: false)
   --stream  keep the subscription open, printing all events as they are returned (default: false, will close on EOSE)

   FILTER ATTRIBUTES

   --author value, -a value [ --author value, -a value ]  only accept events from these authors (pubkey as hex)
   --id value, -i value [ --id value, -i value ]          only accept events with these ids (hex)
   --kind value, -k value [ --kind value, -k value ]      only accept events with these kind numbers
   --limit value, -l value                                only accept up to this number of events (default: 0)
   --since value, -s value                                only accept events newer than this (unix timestamp) (default: 0)
   --tag value, -t value [ --tag value, -t value ]        takes a tag like -t e=<id>, only accept events with these tags
   --until value, -u value                                only accept events older than this (unix timestamp) (default: 0)
   -e value [ -e value ]                                  shortcut for --tag e=<value>
   -p value [ -p value ]                                  shortcut for --tag p=<value>


~> nak decode --help
NAME:
   nak decode - decodes nip19, nip21, nip05 or hex entities

USAGE:
   nak decode [command options] <npub | nprofile | nip05 | nevent | naddr | nsec>

DESCRIPTION:
   example usage:
       nak decode npub1uescmd5krhrmj9rcura833xpke5eqzvcz5nxjw74ufeewf2sscxq4g7chm
       nak decode nevent1qqs29yet5tp0qq5xu5qgkeehkzqh5qu46739axzezcxpj4tjlkx9j7gpr4mhxue69uhkummnw3ez6ur4vgh8wetvd3hhyer9wghxuet5sh59ud
       nak decode nprofile1qqsrhuxx8l9ex335q7he0f09aej04zpazpl0ne2cgukyawd24mayt8gpz4mhxue69uhk2er9dchxummnw3ezumrpdejqz8thwden5te0dehhxarj94c82c3wwajkcmr0wfjx2u3wdejhgqgcwaehxw309aex2mrp0yhxummnw3exzarf9e3k7mgnp0sh5
       nak decode nsec1jrmyhtjhgd9yqalps8hf9mayvd58852gtz66m7tqpacjedkp6kxq4dyxsr

OPTIONS:
   --id, -e      return just the event id, if applicable (default: false)
   --pubkey, -p  return just the pubkey, if applicable (default: false)
   --help, -h    show help


~> nak encode --help
NAME:
   nak encode - encodes notes and other stuff to nip19 entities

USAGE:
   nak encode command [command options] [arguments...]

DESCRIPTION:
   example usage:
       nak encode npub <pubkey-hex>
       nak encode nprofile <pubkey-hex>
       nak encode nprofile --relay <relay-url> <pubkey-hex>
       nak encode nevent <event-id>
       nak encode nevent --author <pubkey-hex> --relay <relay-url> --relay <other-relay> <event-id>
       nak encode nsec <privkey-hex>

COMMANDS:
   npub      encode a hex private key into bech32 'npub' format
   nsec      encode a hex private key into bech32 'nsec' format
   nprofile  generate profile codes with attached relay information
   nevent    generate event codes with optionally attached relay information
   naddr     generate codes for NIP-33 parameterized replaceable events
   help, h   Shows a list of commands or help for one command

OPTIONS:
   --help, -h  show help
```

written in go using [go-nostr](https://github.com/nbd-wtf/go-nostr), heavily inspired by [nostril](http://git.jb55.com/nostril/).

## a toolkit for debugging all things nostr as a webpage:

![](https://user-images.githubusercontent.com/1653275/227681805-0cd20b39-de0d-4fcb-abb4-de3283404e8f.png)

written in [scala](https://scala-lang.org/) with [calico](https://www.armanbilge.com/calico/) and [snow](https://github.com/fiatjaf/snow)
