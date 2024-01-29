# nak, the nostr army knife

install with `go install github.com/fiatjaf/nak@latest` or
[download a binary](https://github.com/fiatjaf/nak/releases).

## what can you do with it?

take a look at the help text that comes in it to learn all possibilities, but here are some:

### make a nostr event signed with the default key (`01`)

```shell
~> nak event
{"id":"53443506e7d09e55b922a2369b80f926007a8a8a8ea5f09df1db59fe1993335e","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1698632644,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"4bdb609c975b2b61338c2ff4c7ce91d4afe74bea4ed1601a62e1fd125bd4c0ae6e0166cca96e5cfb7e0f50583eb6a0dd0b66072566299b6007742db56278010c"}
```

### make a nostr event with custom content and tags, sign it with a different key and publish it to two relays
```shell
~> nak event --sec 02 -c 'good morning' --tag t=gm wss://nostr-pub.wellorder.net wss://relay.damus.io
{"id":"e20978737ab7cd36eca300a65f11738176123f2e0c23054544b18fe493e2aa1a","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698632753,"kind":1,"tags":[["t","gm"]],"content":"good morning","sig":"5687c1a97066c349cb3bde0c0719fd1652a13403ba6aca7557b646307ee6718528cd86989db08bf6a7fd04bea0b0b87c1dd1b78c2d21b80b80eebab7f40b8916"}
publishing to wss://nostr-pub.wellorder.net... success.
publishing to wss://relay.damus.io... success.
```

### query a bunch of relays for a tag with a limit of 2 for each, print their content
```shell
~> nak req -k 1 -t t=gm -l 2 wss://nostr.mom wss://nostr.wine wss://nostr-pub.wellorder.net | jq .content
"#GM, you sovereign savage #freeple of the #nostrverse. Let's cause some #nostroversy. "
"ITM slaves!\n#gm https://image.nostr.build/cbbcdf80bfc302a6678ecf9387c87d87deca3e0e288a12e262926c34feb3f6aa.jpg "
"good morning"
"The problem is to start, but along the way it's fixed #GM ☀️"
"Activando modo zen…\n\n#GM #Nostr #Hispano"
```

### decode a nip19 note1 code, add a relay hint, encode it back to nevent1
```shell
~> nak decode note1ttnnrw78wy0hs5fa59yj03yvcu2r4y0xetg9vh7uf4em39n604vsyp37f2 | jq -r .id | nak encode nevent -r wss://nostr.zbd.gg
nevent1qqs94ee3h0rhz8mc2y76zjf8cjxvw9p6j8nv45zktlwy6uacjea86kgpzfmhxue69uhkummnw3ezu7nzvshxwec8zw8h7
~> nak decode nevent1qqs94ee3h0rhz8mc2y76zjf8cjxvw9p6j8nv45zktlwy6uacjea86kgpzfmhxue69uhkummnw3ezu7nzvshxwec8zw8h7
{
  "id": "5ae731bbc7711f78513da14927c48cc7143a91e6cad0565fdc4d73b8967a7d59",
  "relays": [
    "wss://nostr.zbd.gg"
  ]
}
```

### fetch an event using relay and author hints automatically from a nevent1 code, pretty-print it
```shell
nak fetch nevent1qqs2e3k48vtrkzjm8vvyzcmsmkf58unrxtq2k4h5yspay6vhcqm4wqcpz9mhxue69uhkummnw3ezuamfdejj7q3ql2vyh47mk2p0qlsku7hg0vn29faehy9hy34ygaclpn66ukqp3afqxpqqqqqqz7ttjyq | jq
{
  "id": "acc6d53b163b0a5b3b18416370dd9343f26332c0ab56f42403d26997c0375703",
  "pubkey": "fa984bd7dbb282f07e16e7ae87b26a2a7b9b90b7246a44771f0cf5ae58018f52",
  "created_at": 1697370933,
  "kind": 1,
  "tags": [],
  "content": "`q` tags = a kind 1 that wanted to be a kind:6 but fell short\n\n🥁",
  "sig": "b5b63d7c8491a4a0517df2c58151665c583abc6cd31fd50b957bf8fefc8e55c87c922cbdcb50888cb9f1c03c26ab5c02c1dccc14b46b78e1e16c60094f2358da"
}
```

### republish an event from one relay to multiple others
```shell
~> nak req -i e20978737ab7cd36eca300a65f11738176123f2e0c23054544b18fe493e2aa1a wss://nostr.wine/ wss://nostr-pub.wellorder.net | nak event wss://nostr.wine wss://offchain.pub wss://public.relaying.io wss://eden.nostr.land wss://atlas.nostr.land wss://relayable.org
{"id":"e20978737ab7cd36eca300a65f11738176123f2e0c23054544b18fe493e2aa1a","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698632753,"kind":1,"tags":[["t","gm"]],"content":"good morning","sig":"5687c1a97066c349cb3bde0c0719fd1652a13403ba6aca7557b646307ee6718528cd86989db08bf6a7fd04bea0b0b87c1dd1b78c2d21b80b80eebab7f40b8916"}
publishing to wss://nostr.wine... failed: msg: blocked: not an active paid member
publishing to wss://offchain.pub... success.
publishing to wss://public.relaying.io... success.
publishing to wss://eden.nostr.land... failed: msg: blocked: not on white-list
publishing to wss://atlas.nostr.land... failed: msg: blocked: not on white-list
publishing to wss://relayable.org... success.
```

### verify if an event is good
```shell
~> echo '{"content":"hello world","created_at":1698923350,"id":"05bd99d54cb835f327e0092c4275ee44c7ff51219eff417c19f70c9e2c53ad5a","kind":1,"pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","sig":"0a04a296321ed933858577f36fb2fb9a0933e966f9ee32b539493f5a4d00120891b1ca9152ebfbc04fb403bdaa7c73f415e7c4954e55726b4b4fa8cebf008cd6","tags":[]}' | nak verify
invalid .id, expected 05bd99d54cb835f427e0092c4275ee44c7ff51219eff417c19f70c9e2c53ad5a, got 05bd99d54cb835f327e0092c4275ee44c7ff51219eff417c19f70c9e2c53ad5a
```

### fetch all quoted events by a given pubkey in their last 100 notes
```shell
nak req -l 100 -k 1 -a 2edbcea694d164629854a52583458fd6d965b161e3c48b57d3aff01940558884 wss://relay.damus.io | jq -r '.content | match("nostr:((note1|nevent1)[a-z0-9]+)";"g") | .captures[0].string' | nak decode | jq -cr '{ids: [.id]}' | nak req wss://relay.damus.io
```

## Contributing to this repository

Use NIP-34 to send your patches to `naddr1qqpkucttqy28wumn8ghj7un9d3shjtnwdaehgu3wvfnsz9nhwden5te0wfjkccte9ehx7um5wghxyctwvsq3gamnwvaz7tmjv4kxz7fwv3sk6atn9e5k7q3q80cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsxpqqqpmej2wctpn`.
