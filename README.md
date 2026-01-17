# nak, the nostr army knife

install with `go install github.com/fiatjaf/nak@latest` or
[download a binary](https://github.com/fiatjaf/nak/releases).

or get the source with `git clone https://github.com/fiatjaf/nak` then install with `go install` or run with docker using `docker build -t nak . && docker run nak event`.

## what can you do with it?

take a look at the help text that comes in it to learn all possibilities, but here are some:

### make a nostr event signed with the default key (`01`)

```shell
~> nak event
{"id":"53443506e7d09e55b922a2369b80f926007a8a8a8ea5f09df1db59fe1993335e","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1698632644,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"4bdb609c975b2b61338c2ff4c7ce91d4afe74bea4ed1601a62e1fd125bd4c0ae6e0166cca96e5cfb7e0f50583eb6a0dd0b66072566299b6007742db56278010c"}
```

### make a nostr event with custom content and tags, sign it with a different key and publish it to two relays
```shell
~> nak event --sec 02 -c 'good morning' --tag t=gm nostr-pub.wellorder.net relay.damus.io
{"id":"e20978737ab7cd36eca300a65f11738176123f2e0c23054544b18fe493e2aa1a","pubkey":"c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5","created_at":1698632753,"kind":1,"tags":[["t","gm"]],"content":"good morning","sig":"5687c1a97066c349cb3bde0c0719fd1652a13403ba6aca7557b646307ee6718528cd86989db08bf6a7fd04bea0b0b87c1dd1b78c2d21b80b80eebab7f40b8916"}
publishing to wss://nostr-pub.wellorder.net... success.
publishing to wss://relay.damus.io... success.
```

### query a bunch of relays for a tag with a limit of 2 for each, print their content
```shell
~> nak req -k 1 -t t=gm -l 2 nostr.mom nostr.wine nostr-pub.wellorder.net | jq .content
"#GM, you sovereign savage #freeple of the #nostrverse. Let's cause some #nostroversy. "
"ITM slaves!\n#gm https://image.nostr.build/cbbcdf80bfc302a6678ecf9387c87d87deca3e0e288a12e262926c34feb3f6aa.jpg "
"good morning"
"The problem is to start, but along the way it's fixed #GM ☀️"
"Activando modo zen…\n\n#GM #Nostr #Hispano"
```

### decode a nip19 note1 code, add a relay hint, encode it back to nevent1
```shell
~> nak decode note1ttnnrw78wy0hs5fa59yj03yvcu2r4y0xetg9vh7uf4em39n604vsyp37f2 | jq -r .id | nak encode nevent -r nostr.zbd.gg
nevent1qqs94ee3h0rhz8mc2y76zjf8cjxvw9p6j8nv45zktlwy6uacjea86kgpzfmhxue69uhkummnw3ezu7nzvshxwec8zw8h7
~> nak decode nevent1qqs94ee3h0rhz8mc2y76zjf8cjxvw9p6j8nv45zktlwy6uacjea86kgpzfmhxue69uhkummnw3ezu7nzvshxwec8zw8h7
{
  "id": "5ae731bbc7711f78513da14927c48cc7143a91e6cad0565fdc4d73b8967a7d59",
  "relays": [
    "nostr.zbd.gg"
  ]
}
```

### fetch all events except those that are present in a given line-delimited json file (negentropy sync)
```shell
~> nak req --only-missing ./events.jsonl -k 30617 pyramid.fiatjaf.com
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
~> nak req -i e20978737ab7cd36eca300a65f11738176123f2e0c23054544b18fe493e2aa1a nostr.wine/ nostr-pub.wellorder.net | nak event nostr.wine offchain.pub public.relaying.io eden.nostr.land atlas.nostr.land relayable.org
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

### fetch all quoted events by a given pubkey in their last 10 notes of 2023
```shell
~> nak req -l 10 -k 1 --until 'December 31 2023' -a 2edbcea694d164629854a52583458fd6d965b161e3c48b57d3aff01940558884 relay.damus.io | jq -r '.content | match("nostr:((note1|nevent1)[a-z0-9]+)";"g") | .captures[0].string' | nak decode | jq -cr '{ids: [.id]}' | nak req relay.damus.io
connecting to relay.damus.io...
ok.
{"kind":1,"id":"0000000a5109c9747e3847282fcaef3d221d1be5e864ced7b2099d416a18d15a","pubkey":"7bdef7be22dd8e59f4600e044aa53a1cf975a9dc7d27df5833bc77db784a5805","created_at":1703869609,"tags":[["nonce","12912720851599460299","25"]],"content":"https://image.nostr.build/5eb40d3cae799bc572763b8f8bee95643344fa392d280efcb0fd28a935879e2a.png\n\nNostr is not dying.\nIt is just a physiological and healthy slowdown on the part of all those who have made this possible in such a short time, sharing extraordinary enthusiasm. This is necessary to regain a little energy, it will allow some things to be cleaned up and more focused goals to be set.\n\nIt is like the caterpillar that is about to become a butterfly, it has to stop moving, acting, doing all the time; it has to do one last silent work and rest, letting time go by. And then a new phase of life can begin.\n\nWe have an amazing 2024 ahead.\nThank you all, who have given so much and believe in Nostr.\n\nPS: an interesting cue suggested by this image, you cannot have both silk and butterfly, you have to choose: a precious and sophisticated ornament, or the living, colorful beauty of freedom.","sig":"16fe157fb13dba2474d510db5253edc409b465515371015a91b26b8f39e5aa873453bc366947c37463c49466f5fceb7dea0485432f979a03471c8f76b73e553c"}
{"kind":1,"id":"ac0cc72dfee39f41d94568f574e7b613d3979facbd7b477a16b52eb763db4b6e","pubkey":"2250f69694c2a43929e77e5de0f6a61ae5e37a1ee6d6a3baef1706ed9901248b","created_at":1703873865,"tags":[["r","https://zine.wavlake.com/2023-year-in-review/"]],"content":"It's been an incredible year for us here at Wavlake and we wanted to take a moment to look back and see how far we've come since launch. Read more.. https://zine.wavlake.com/2023-year-in-review/","sig":"189e354f67f48f3046fd762c83f9bf3a776d502d514e2839a1b459c30107a02453304ef695cdc7d254724041feec3800806b21eb76259df87144aaef821ace5b"}
{"kind":1,"id":"6215766c5aadfaf51488134682f7d28f237218b5405da2fc11d1fefe1ebf8154","pubkey":"4ce6abbd68dab6e9fdf6e8e9912a8e12f9b539e078c634c55a9bff2994a514dd","created_at":1703879775,"tags":[["imeta","url https://video.nostr.build/7b4e7c326fa4fcba58a40914ce9db4f060bd917878f2194f6d139948b085ebb9.mp4","blurhash eHD,QG_4ogMu_3to%O-:MwM_IWRjx^-pIUoe-;t7%Nt7%gV?M{WBxu","dim 480x268"],["t","zaps"],["t","powakili23"],["p","4f82bced42584a6acfced2a657b5acabc4f90d75a95ed3ff888f3b04b8928630"],["p","ce75bae2349804caa5f4de8ae8f775bb558135f412441d9e32f88e4226c5d165"],["p","94bd495b78f8f6e5aff8ebc90e052d3a409d1f9d82e43ab56ca2cafb81b18ddf"],["p","50ff5b7ebeac1cc0d03dc878be8a59f1b63d45a7d5e60ade4b6f6f31eca25954"],["p","f300cf2bdf9808ed229dfa468260753a0b179935bdb87612b6d4f5b9fe3fc7cf"],["r","https://geyser.fund/entry/2636"],["r","https://video.nostr.build/7b4e7c326fa4fcba58a40914ce9db4f060bd917878f2194f6d139948b085ebb9.mp4"]],"content":"POWA - HQ UPDATE - DEC 2023\nTLDR: plan to open January 2024, 1 million Sats to go to reach milestone. #zaps go to fund this project. ⚡️powa@geyser.fund\n\nHello,\n\nFirst and foremost, I’d like to thank you for the incredible support shown for this project. It’s been an absolute honor to oversee this Proof of Work initiative.\n\nI am thrilled to announce that we are right on track for the grand opening in January 2024.\n\nCurrently, we're just over 1 million Sats away from reaching our target for this phase.\n\nPlease take a moment to enjoy the video and stay tuned for further updates about POWA. \n\nMan Like Who?\nMan Like Kweks!\n🇹🇿⚡️💜🏔️\n#powakili23\nnostr:npub1f7ptem2ztp9x4n7w62n90ddv40z0jrt4490d8lug3uasfwyjsccqkknerm nostr:npub1ee6m4c35nqzv4f05m69w3am4hd2czd05zfzpm83jlz8yyfk969js78tfcv nostr:npub1jj75jkmclrmwttlca0ysupfd8fqf68uastjr4dtv5t90hqd33h0s4gcksp nostr:npub12rl4kl474swvp5paeputazje7xmr63d86hnq4hjtdahnrm9zt92qgq500s nostr:npub17vqv727lnqyw6g5alfrgycr48g930xf4hku8vy4k6n6mnl3lcl8sglecc5 \n\nhttps://geyser.fund/entry/2636 https://video.nostr.build/7b4e7c326fa4fcba58a40914ce9db4f060bd917878f2194f6d139948b085ebb9.mp4 ","sig":"97d13c17d91c319f343cc770222d6d4a0a714d0e7e4ef43373adaf215a4c077f0bdf12bac488c74dbd4d55718d46c17a617b93c8660736b70bcd61a8820ece67"}
# and so on...
```

### sign an event collaboratively with multiple parties using musig2
```shell
~> nak event --sec 1234 -k 1 -c 'hello from a combined key' --musig 2
the following code should be saved secretly until the next step an included with --musig-nonce-secret:
QebOT03ERmV7km22CqEqBPFmzAkgxQzGGbR7Si8yIZCBrd1N9A3LKwGLO71kbgXZ9EYFKpjiwun4u0mj5Tq6vwM3pK7x+EI8oHbkt9majKv/QN24Ix8qnwEIHxXX+mXBug==

the next signer and they should call this on their side:
nak event --sec <insert-secret-key> --musig 2 -k 1 -ts 1720821287 -c 'hello from a combined key' --musig-pubkey 0337a4aef1f8423ca076e4b7d99a8cabff40ddb8231f2a9f01081f15d7fa65c1ba --musig-nonce 0285af37c6c43638cda2c773098e867c749ddf1e9d096b78686c5d000603935ad3025c4a1e042eb6b0dcfd864d1e072d2ce8da06f2c0dcf13fd7d1fcef0dd26dbc92
```

demo videos with [2](https://njump.me/nevent1qqs8pmmae89agph80928l6gjm0wymechqazv80jwqrqy4cgk08epjaczyqalp33lewf5vdq847t6te0wvnags0gs0mu72kz8938tn24wlfze674zkzz), [3](https://njump.me/nevent1qqsrp320drqcnmnam6jvmdd4lgdvh2ay0xrdesrvy6q9qqdfsk7r55qzyqalp33lewf5vdq847t6te0wvnags0gs0mu72kz8938tn24wlfze6c32d4m) and [4](https://njump.me/nevent1qqsre84xe6qpagf2w2xjtjwc95j4dd5ccue68gxl8grkd6t6hjhaj5qzyqalp33lewf5vdq847t6te0wvnags0gs0mu72kz8938tn24wlfze6t8t7ak) parties.

### generate a private key
```shell
~> nak key generate
7b94e287b1fafa694ded1619b27de7effd3646104a158e187ff4edc56bc6148d
```

### encrypt key with NIP-49
```shell
~> nak key encrypt 7b94e287b1fafa694ded1619b27de7effd3646104a158e187ff4edc56bc6148d mypassword
ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls
```

### decrypt key with NIP-49
```shell
~> nak key decrypt ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls mypassword
7b94e287b1fafa694ded1619b27de7effd3646104a158e187ff4edc56bc6148d
~>
~> nak key decrypt ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls
type the password to decrypt your secret key: **********
7b94e287b1fafa694ded1619b27de7effd3646104a158e187ff4edc56bc6148d
```

### get a public key from a private key
```shell
~> nak key public 7b94e287b1fafa694ded1619b27de7effd3646104a158e187ff4edc56bc6148d
985d66d2644dfa7676e26046914470d66ebc7fa783a3f57f139fde32d0d631d7
```

### sign an event using a bunker provider (amber, promenade etc)
```shell
~> export NOSTR_CLIENT_KEY="$(nak key generate)"
~> nak event --sec 'bunker://a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208?relay=wss%3A%2F%2Frelay.damus.io&relay=wss%3A%2F%2Frelay.nsecbunker.com&relay=wss%3A%2F%2Fnos.lol&secret=TWfGbjQCLxUf' -c 'hello from bunker'
```

(in most cases it's better to set `NOSTR_CLIENT_KEY` permanently on your shell, as that identity will be recorded by the bunker provider.)

### sign an event using a NIP-49 encrypted key
```shell
~> nak event --sec ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls -c 'hello from encrypted key'
type the password to decrypt your secret key: **********
{"kind":1,"id":"8aa5c931fb1da507f14801de6a1814b7f0baae984dc502b9889f347f5aa3cc4e","pubkey":"985d66d2644dfa7676e26046914470d66ebc7fa783a3f57f139fde32d0d631d7","created_at":1720822280,"tags":[],"content":"hello from encrypted key","sig":"9d1c9e56e87f787cc5b6191ec47690ce59fa4bef105b56297484253953e18fb930f6683f007e84a9ce9dc9a25b20c191c510629156dcd24bd16e15d302d20944"}
```

### talk to a relay's NIP-86 management API
```shell
nak admin allowpubkey --sec ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls --pubkey a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208 pyramid.fiatjaf.com
type the password to decrypt your secret key: **********
calling 'allowpubkey' on https://pyramid.fiatjaf.com...
{
  "result": null,
  "error": "failed to add to whitelist: pubkey 985d66d2644dfa7676e26046914470d66ebc7fa783a3f57f139fde32d0d631d7 doesn't have permission to invite"
}
```

### start a bunker locally
```shell
~> nak bunker --sec ncryptsec1qggrp80ptf0s7kyl0r38ktzg60fem85m89uz7um6rjn4pnep2nnvcgqm8h7q36c76z9sypatdh4fmw6etfxu99mv5cxkw4ymcsryw0zz7evyuplsgvnj5yysf449lq94klzvnahsw2lzxflvcq4qpf5q -k 3fbf7fbb2a2111e205f74aca0166e29e421729c9a07bc45aa85d39535b47c9ed relay.damus.io nos.lol relay.nsecbunker.com
connecting to relay.damus.io... ok.
connecting to nos.lol... ok.
connecting to relay.nsecbunker.com... ok.
type the password to decrypt your secret key: ***
listening at [wss://relay.damus.io wss://nos.lol wss://relay.nsecbunker.com]:
  pubkey: f59911b561c37c90b01e9e5c2557307380835c83399756f4d62d8167227e420a
  npub: npub17kv3rdtpcd7fpvq7newz24eswwqgxhyr8xt4daxk9kqkwgn7gg9q4gy8vf
  authorized keys:
    - 3fbf7fbb2a2111e205f74aca0166e29e421729c9a07bc45aa85d39535b47c9ed
  to restart: nak bunker --sec ncryptsec1qggrp80ptf0s7kyl0r38ktzg60fem85m89uz7um6rjn4pnep2nnvcgqm8h7q36c76z9sypatdh4fmw6etfxu99mv5cxkw4ymcsryw0zz7evyuplsgvnj5yysf449lq94klzvnahsw2lzxflvcq4qpf5q -k 3fbf7fbb2a2111e205f74aca0166e29e421729c9a07bc45aa85d39535b47c9ed relay.damus.io nos.lol relay.nsecbunker.com
  bunker: bunker://f59911b561c37c90b01e9e5c2557307380835c83399756f4d62d8167227e420a?relay=wss%3A%2F%2Frelay.damus.io&relay=wss%3A%2F%2Fnos.lol&relay=wss%3A%2F%2Frelay.nsecbunker.com&secret=XuuiMbcLwuwL
```

you can also display a QR code for the bunker URI by adding the `--qrcode` flag:

```shell
~> nak bunker --qrcode --sec ncryptsec1... relay.damus.io
```

### start a bunker that persists its metadata (secret key, relays, authorized client pubkeys) to disc
```shell
~> nak bunker --persist --sec ncryptsec1... relay.nsec.app nos.lol
```

then later just

```shell
~> nak bunker --persist
```

or give it a named profile:

```shell
~> nak bunker --profile myself ...
```

### generate a NIP-70 protected event with a date set to two weeks ago and some multi-value tags
```shell
~> nak event --ts 'two weeks ago' -t '-' -t 'e=f59911b561c37c90b01e9e5c2557307380835c83399756f4d62d8167227e420a;wss://relay.whatever.com;root;a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208' -t 'p=a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208;wss://p-relay.com' -c 'I know the future'
{"kind":1,"id":"f030fccd90c783858dfcee204af94826cf0f1c85d6fc85a0087e9e5172419393","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1719677535,"tags":[["-"],["e","f59911b561c37c90b01e9e5c2557307380835c83399756f4d62d8167227e420a","wss://relay.whatever.com","root","a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208"],["p","a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208","wss://p-relay.com"]],"content":"I know the future","sig":"8b36a74e29df8bc12bed66896820da6940d4d9409721b3ed2e910c838833a178cb45fd5bb1c6eb6adc66ab2808bfac9f6644a2c55a6570bb2ad90f221c9c7551"}
```

### download the latest 50000 notes from a relay, regardless of their natural query limits, by paginating requests
```shell
~> nak req -k 1 --limit 50000 --paginate --paginate-interval 2s nos.lol > events.jsonl
~> wc -l events.jsonl
50000 events.jsonl
```

### run a somewhat verbose local relay for test purposes
```shell
~> nak serve
> relay running at ws://localhost:10547
    got request {"kinds":[1],"authors":["79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"],"since":1724082362}
    got event {"kind":1,"id":"e3c6bf630d6deea74c0ee2f7f7ba6da55a627498a32f1e72029229bb1810bce3","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1724082366,"tags":[],"content":"two","sig":"34261cf226c3fee2df24e55a89f43f5349c98a64bce46bdc46807b0329f334cea93e9e8bc285c1259a5684cf23f5e507c8e6dad47a31a6615d706b1130d09e69"}
    got event {"kind":1,"id":"0bbb397c8f87ae557650b9d6ee847292df8e530c458ffea1b24bdcb7bed0ec5e","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1724082369,"tags":[],"content":"three","sig":"aa1cb7d5f0f03f358fc4c0a4351a4f1c66e3a7627021b618601c56ba598b825b6d95d9c8720a4c60666a7eb21e17018cf326222f9f574a9396f2f2da7f007546"}
  • events stored: 2, subscriptions opened: 1
    got event {"kind":1,"id":"029ebff759dd54dbd01b929f879fea5802de297e1c3768ca16d9b97cc8bca38f","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1724082371,"tags":[],"content":"four","sig":"9816de517d87d4c3ede57c1c50e3c237486794241afadcd891e1acbba2c5e672286090e6ad3402b047d69bae8095bc4e20e57ac70d92386dfa26db216379330f"}
    got event {"kind":1,"id":"fe6489fa6fbb925be839377b9b7049d73be755dc2bdad97ff6dd9eecbf8b3a32","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1724082383,"tags":[],"content":"five","sig":"865ce5e32eead5bdb950ac1fbc55bc92dde26818ee3136634538ec42914de179a51e672c2d4269d4362176e5e8cd5e08e69b35b91c6c2af867e129b93d607635"}
    got request {"kinds":[30818]}
  • events stored: 4, subscriptions opened: 1
```

### enable negentropy (nip77) support in your development relay
```shell
~> nak serve --negentropy
```

### run a grasp server (with a relay)
```shell
~> nak serve --grasp
```

### run a blossom server (with a relay)
```shell
~> nak serve --blossom
```

### make an event with a PoW target
```shell
~> nak event -c 'hello getwired.app and labour.fiatjaf.com' --pow 24
{"kind":1,"id":"0000009dcc7c62056eafdb41fac817379ec2becf0ce27c5fbe98d0735d968147","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1724160828,"tags":[["nonce","515504","24"]],"content":"hello getwired.app and labour.fiatjaf.com","sig":"7edb988065ccc12779fe99270945b212f3723838f315d76d5e90e9ffa27198f13fa556614295f518d968d55bab81878167d4162b3a7cf81a6b423c6761bd504c"}
```

### make a nostr event signed with a key given as an environment variable

```shell
~> export NOSTR_SECRET_KEY=ncryptsec1qggyy9vw0nclmw8ly9caz6aa7f85a4ufhsct64uva337pulsdw00n6twa2lzhzk2znzsyu60urx9s08lx00ke6ual3lszyn5an9zarm6s70lw5lj6dv3mj3f9p4tvp0we6qyz4gp420mapfmvqheuttv
~> nak event -c 'it supports keys as hex, nsec or ncryptsec'
type the password to decrypt your secret key: ********
{"kind":1,"id":"5cbf3feb9a7d99c3ee2a88693a591caca1a8348fea427b3652c27f7a8a76af48","pubkey":"b00bcab55375d8c7b731dd9841f6d805ff1cf6fdc945e7326786deb5ddac6ce4","created_at":1724247924,"tags":[],"content":"it supports keys as hex, nsec or ncryptsec","sig":"fb3fd170bc10e5042322c7a05dd4bbd8ac9947b39026b8a7afd1ee02524e8e3aa1d9554e9c7b6181ca1b45cab01cd06643bdffa5ce678b475e6b185e1c14b085"}
```

### download some helpful `jq` functions for dealing with nostr events
```shell
~> nak req -i 412f2d3e73acc312942c055ac2a695dc60bf58ff97e06689a8a79e97796c4cdb relay.westernbtc.com | jq -r .content > ~/.jq
```

### watch a NIP-53 livestream (zap.stream, amethyst, shosho etc)
```shell
~> # this requires the jq utils from the step above
~> mpv $(nak fetch naddr1qqjxvvm9xscnsdtx95cxvcfk956rsvtx943rje3k95mx2dp389jnwwrp8ymxgqg4waehxw309aex2mrp0yhxgctdw4eju6t09upzpn6956apxcad0mfp8grcuugdysg44eepex68h50t73zcathmfs49qvzqqqrkvu7ed38k | jq -r 'tag_value("streaming")')
~>
~> # or without the utils
~> mpv $(nak fetch naddr1qqjxvvm9xscnsdtx95cxvcfk956rsvtx943rje3k95mx2dp389jnwwrp8ymxgqg4waehxw309aex2mrp0yhxgctdw4eju6t09upzpn6956apxcad0mfp8grcuugdysg44eepex68h50t73zcathmfs49qvzqqqrkvu7ed38k | jq -r '.tags | map(select(.[0] == "streaming") | .[1])[0]')
```

### download a NIP-35 torrent from an `nevent`
```shell
~> # this requires the jq utils from two steps above
~> aria2c $(nak fetch nevent1qqsdsg6x7uujekac4ga7k7qa9q9sx8gqj7xzjf5w9us0dm0ghvf4ugspp4mhxue69uhkummn9ekx7mq6dw9y4 | jq -r '"magnet:?xt=urn:btih:\(tag_value("x"))&dn=\(tag_value("title"))&tr=http%3A%2F%2Ftracker.loadpeers.org%3A8080%2FxvRKfvAlnfuf5EfxTT5T0KIVPtbqAHnX%2Fannounce&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A6969%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.torrent.eu.org%3A451%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=\(tags("tracker") | map(.[1] | @uri) | join("&tr="))"')
```

### mount Nostr as a FUSE filesystem and publish a note
```shell
~> nak fs --sec 01 ~/nostr
- mounting at /home/user/nostr... ok.
~> cd ~/nostr/npub1xxxxxx/notes/
~> echo "satellites are bad!" > new
pending note updated, timer reset.
- `touch publish` to publish immediately
- `rm new` to erase and cancel the publication.
~> touch publish
publishing now!
{"id":"f1cbfa6...","pubkey":"...","content":"satellites are bad!","sig":"..."}
publishing to 3 relays... offchain.pub: ok, nostr.wine: ok, pyramid.fiatjaf.com: ok
event published as f1cbfa6... and updated locally.
```

### list NIP-60 wallet tokens and send some
```shell
~> nak wallet tokens
91a10b6fc8bbe7ef2ad9ad0142871d80468b697716d9d2820902db304ff1165e 500 cashu.space
cac7f89f0611021984d92a7daca219e4cd1c9798950e50e952bba7cde1ac1337 1000 legend.lnbits.com
~> nak wallet send 100
cashuA1psxqyry8...
~> nak wallet pay lnbc1...
```

### upload and download files with blossom
```shell
~> nak blossom --server blossom.azzamo.net --sec 01 upload image.png
{"sha256":"38c51756f3e9fedf039488a1f6e513286f6743194e7a7f25effdc84a0ee4c2cf","url":"https://blossom.azzamo.net/38c51756f3e9fedf039488a1f6e513286f6743194e7a7f25effdc84a0ee4c2cf.png"}
~> nak blossom --server aegis.utxo.one download acc8ea43d4e6b706f68b249144364f446854b7f63ba1927371831c05dcf0256c -o downloaded.png
```

### publish a fully formed event with correct tags, URIs and to the correct read and write relays
```shell
echo "#surely you're joking, mr npub1l2vyh47mk2p0qlsku7hg0vn29faehy9hy34ygaclpn66ukqp3afqutajft olas.app is broken again" | nak publish

# it will add the hashtag, turn the npub1 code into a nostr:npub1 URI, turn the olas.app string into https://olas.app, add the "p" tag (and "q" tags too if you were mentioning an nevent1 code or naddr1 code) and finally publish it to your "write" relays and to any mentioned person (or author of mentioned events)'s "read" relays.
# there is also a --reply flag that you can pass an nevent, naddr or hex id to and it will do the right thing (including setting the correct kind to either 1 or 1111).
# and there is a --confirm flag that gives you a chance to confirm before actually publishing the result to relays.
```

### record and publish an audio note (yakbak, nostur etc) signed from a bunker
```shell
ffmpeg -f alsa -i default -f webm -t 00:00:03 pipe:1 | nak blossom --server blossom.primal.net upload | jq -rc '{content: .url}' | nak event -k 1222 --sec 'bunker://urlgoeshere' pyramid.fiatjaf.com nostr.wine
```

### gift-wrap an event to a recipient and publish it somewhere
```shell
~> nak event -c 'secret message' | nak gift wrap --sec <my-secret-key> -p <recipient-public-key> | nak event wss://dmrelay.com
```

### download a gift-wrap event and unwrap it
```shell
~> nak req -p <my-public-key> -k 1059 relay.com | nak gift unwrap --sec <my-secret-key> --from <sender-public-key>
```

### sync events between two relays using negentropy
```shell
~> nak sync relay1.com relay2.com
```

### get nak to be very verbose about all messages sent and received to relays
```shell
~> go install -tags=debug github.com/fiatjaf/nak@latest
~> #
~> # then, for example:
~> nak req -k 30617 -k 30618 pyramid.treegaze.com gitnostr.com -a bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747
```

<details>
<summary>output (mixing stdin and stderr)</summary>
<pre>
<code>
pyramid.treegaze.com... ok.
gitnostr.com... ok.
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} sending '["REQ","1:nak-req",{"kinds":[30617,30618],"authors":["bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"]}]'
[nl][debug] 2025/12/05 22:00:53 {wss://pyramid.treegaze.com} sending '["REQ","2:nak-req",{"kinds":[30617,30618],"authors":["bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"]}]'
[nl][debug] 2025/12/05 22:00:53 {wss://pyramid.treegaze.com} received ["EVENT","2:nak-req",{"kind":30618,"id":"001d5525ef7b529a40a8c1a74a1a21bbe8cf6b525a82814ea71438452ae4159e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973164,"tags":[["d","loom-worker"],["HEAD","ref: refs/heads/main"],["refs/heads/main","fd4fd997ab0c671ad1eb5218f7d37e7fdc6f9e96"]],"content":"","sig":"29cd9542bab9e1fcae1f0dcf296b7874edfca3b5a55472fad5a3079925f40a69fbcacd9dbfa948b6f625297fa2bd5f7aca6639f2cc19015843faab676dbf90ec"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"001d5525ef7b529a40a8c1a74a1a21bbe8cf6b525a82814ea71438452ae4159e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973164,"tags":[["d","loom-worker"],["HEAD","ref: refs/heads/main"],["refs/heads/main","fd4fd997ab0c671ad1eb5218f7d37e7fdc6f9e96"]],"content":"","sig":"29cd9542bab9e1fcae1f0dcf296b7874edfca3b5a55472fad5a3079925f40a69fbcacd9dbfa948b6f625297fa2bd5f7aca6639f2cc19015843faab676dbf90ec"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"ba4b194e2611946752383ffc8fd1ea225305527c29193d86f8556b66e1837bf7","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973163,"tags":[["d","loom-worker"],["r","7ad47289a1013edcd7778af2dd8c70063f756f7b","euc"],["name","loom-worker"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.fiatjaf.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-worker"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://pyramid.treegaze.com","wss://pyramid.fiatjaf.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-worker"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com","https://pyramid.fiatjaf.com"]],"content":"","sig":"8f3317d0754fca792797d9d2e293f801b4cc5927a40d39eeb0d1b8a4fbb18db8ee0017b1727d663fce39535e5b3afb6f8757b7643516d1c97b3753304920e0b0"}]
[nl][debug] 2025/12/05 22:00:53 {wss://pyramid.treegaze.com} received ["EVENT","2:nak-req",{"kind":30617,"id":"ba4b194e2611946752383ffc8fd1ea225305527c29193d86f8556b66e1837bf7","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973163,"tags":[["d","loom-worker"],["r","7ad47289a1013edcd7778af2dd8c70063f756f7b","euc"],["name","loom-worker"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.fiatjaf.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-worker"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://pyramid.treegaze.com","wss://pyramid.fiatjaf.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-worker"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com","https://pyramid.fiatjaf.com"]],"content":"","sig":"8f3317d0754fca792797d9d2e293f801b4cc5927a40d39eeb0d1b8a4fbb18db8ee0017b1727d663fce39535e5b3afb6f8757b7643516d1c97b3753304920e0b0"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"e3a6c1031a929a6b8a9f7bad51a067968d76fa1f9631920b44caef45e98cbc04","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764971638,"tags":[["d","loom-site"],["HEAD","ref: refs/heads/main"],["refs/heads/main","c32a42e92337e8b6f6c6db0eb015877976800098"]],"content":"","sig":"4ed682dd5c4dc77bec34dc15ae9f60362f2c2ea73ed24e38266be4cee4af5f8959806c8e9fc6cc6266ea6be0c6a77a3c878af1bf4207caec9e7f38399654a0e0"}]
[nl][debug] 2025/12/05 22:00:53 {wss://pyramid.treegaze.com} received ["EOSE","2:nak-req"]
{"kind":30618,"id":"001d5525ef7b529a40a8c1a74a1a21bbe8cf6b525a82814ea71438452ae4159e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973164,"tags":[["d","loom-worker"],["HEAD","ref: refs/heads/main"],["refs/heads/main","fd4fd997ab0c671ad1eb5218f7d37e7fdc6f9e96"]],"content":"","sig":"29cd9542bab9e1fcae1f0dcf296b7874edfca3b5a55472fad5a3079925f40a69fbcacd9dbfa948b6f625297fa2bd5f7aca6639f2cc19015843faab676dbf90ec"}
{"kind":30617,"id":"ba4b194e2611946752383ffc8fd1ea225305527c29193d86f8556b66e1837bf7","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764973163,"tags":[["d","loom-worker"],["r","7ad47289a1013edcd7778af2dd8c70063f756f7b","euc"],["name","loom-worker"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git","https://pyramid.fiatjaf.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-worker.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-worker"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://pyramid.treegaze.com","wss://pyramid.fiatjaf.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-worker"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com","https://pyramid.fiatjaf.com"]],"content":"","sig":"8f3317d0754fca792797d9d2e293f801b4cc5927a40d39eeb0d1b8a4fbb18db8ee0017b1727d663fce39535e5b3afb6f8757b7643516d1c97b3753304920e0b0"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"30a2bb8fdd21e89f4fe329c3e3c47b1597daea33d905fd679754945d63067451","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764723101,"tags":[["d","loom-site"],["r","23c628196368089bb28a249c2e31d14c357b5031","euc"],["name","loom-site"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-site"],["relays","wss://relay.ngit.dev","wss://pyramid.treegaze.com","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-site"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com"]],"content":"","sig":"81ba665abe0195f07dfcfe4f5a306c7c4d1ccf5dcb0ffd270f07c96a98695142cb934fd52246c85d2e3e24d20bcce7630a3433b05324e1f8022f334ce2c6846b"}]
{"kind":30618,"id":"e3a6c1031a929a6b8a9f7bad51a067968d76fa1f9631920b44caef45e98cbc04","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764971638,"tags":[["d","loom-site"],["HEAD","ref: refs/heads/main"],["refs/heads/main","c32a42e92337e8b6f6c6db0eb015877976800098"]],"content":"","sig":"4ed682dd5c4dc77bec34dc15ae9f60362f2c2ea73ed24e38266be4cee4af5f8959806c8e9fc6cc6266ea6be0c6a77a3c878af1bf4207caec9e7f38399654a0e0"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"033e63e83a7ed21de8ae55c13d9c8e9ae378ab9db870773d9c224f71be9db608","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764612202,"tags":[["d","loom-protocol"],["HEAD","ref: refs/heads/main"],["refs/heads/main","97d1656c1fdaa9a22abbbe220966a8a4397a5bbe"]],"content":"","sig":"873c2fcc9edbca85f19020e52bc894881b18b904edec605eb490cec34df2ff541520ea1bc7ca88b3bb8bc91f527f614baf96280e679c5d26822678258c96c28e"}]
{"kind":30617,"id":"30a2bb8fdd21e89f4fe329c3e3c47b1597daea33d905fd679754945d63067451","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764723101,"tags":[["d","loom-site"],["r","23c628196368089bb28a249c2e31d14c357b5031","euc"],["name","loom-site"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-site.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-site"],["relays","wss://relay.ngit.dev","wss://pyramid.treegaze.com","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-site"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com"]],"content":"","sig":"81ba665abe0195f07dfcfe4f5a306c7c4d1ccf5dcb0ffd270f07c96a98695142cb934fd52246c85d2e3e24d20bcce7630a3433b05324e1f8022f334ce2c6846b"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"3fbb928f9f09f5713f7fc75aa5d6177fa127d34fcb098a445a2a70f20715070d","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764612187,"tags":[["d","loom-protocol"],["r","d100e867498703c46f752b815e2dbf0d3e01e0fb","euc"],["name","loom-protocol"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-protocol"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://pyramid.treegaze.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-protocol"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com"]],"content":"","sig":"02776d505592095a3d1c08e8e7f1e0406df75dcdbb8a77926ee3177307f567276a81ad755622f25c81407755921e3f4662ca5277f0a5d32c7e1d467d53d08b6e"}]
{"kind":30618,"id":"033e63e83a7ed21de8ae55c13d9c8e9ae378ab9db870773d9c224f71be9db608","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764612202,"tags":[["d","loom-protocol"],["HEAD","ref: refs/heads/main"],["refs/heads/main","97d1656c1fdaa9a22abbbe220966a8a4397a5bbe"]],"content":"","sig":"873c2fcc9edbca85f19020e52bc894881b18b904edec605eb490cec34df2ff541520ea1bc7ca88b3bb8bc91f527f614baf96280e679c5d26822678258c96c28e"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"565481a335838a3f241b0063602451e100ca3d15ddcbd0b98a4253ab43f40c3e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764609644,"tags":[["d","noflow"],["HEAD","ref: refs/heads/main"],["refs/heads/main","b5d3cdf5708712484a14c41d7f92a59f3161dec6"]],"content":"","sig":"b8ff2848a57ef05f7cd1aff03451b0e6500bd73cc1cdeb4ad49c5080b447a9696401e06983ff36f9e3274b055045ba7b0505e328be116dfd8881f8c1ab084fdc"}]
{"kind":30617,"id":"3fbb928f9f09f5713f7fc75aa5d6177fa127d34fcb098a445a2a70f20715070d","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764612187,"tags":[["d","loom-protocol"],["r","d100e867498703c46f752b815e2dbf0d3e01e0fb","euc"],["name","loom-protocol"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git","https://pyramid.treegaze.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/loom-protocol.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/loom-protocol"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://pyramid.treegaze.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-protocol"],["blossoms","https://relay.ngit.dev","https://gitnostr.com","https://pyramid.treegaze.com"]],"content":"","sig":"02776d505592095a3d1c08e8e7f1e0406df75dcdbb8a77926ee3177307f567276a81ad755622f25c81407755921e3f4662ca5277f0a5d32c7e1d467d53d08b6e"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"f66cc26f7693316db05cff6789333b9312468474a10b2511d439b51ecc0a1436","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764609631,"tags":[["d","noflow"],["r","aaf1722fde2f125862eb3353bb1810f0f653042f","euc"],["name","loom-protocol"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noflow.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noflow.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/noflow"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-protocol"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"05f8381b9cd4dfa0ca564f618a1c39a05bea452867a2208f4f5bf6b51968589eb92728748e175b81c1934ae8275a3e2dd16dec663684abed64f5e7d994e622cb"}]
{"kind":30618,"id":"565481a335838a3f241b0063602451e100ca3d15ddcbd0b98a4253ab43f40c3e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764609644,"tags":[["d","noflow"],["HEAD","ref: refs/heads/main"],["refs/heads/main","b5d3cdf5708712484a14c41d7f92a59f3161dec6"]],"content":"","sig":"b8ff2848a57ef05f7cd1aff03451b0e6500bd73cc1cdeb4ad49c5080b447a9696401e06983ff36f9e3274b055045ba7b0505e328be116dfd8881f8c1ab084fdc"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"cf80564e447768bfe0562f09e9ecacfb4f6c1f125f201b523ad81e56fd5fd8ac","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764343728,"tags":[["d","nutsucker-2000"],["refs/heads/main","a113781b8a32eaab64c44ebc68d3447f91fa4bec"],["HEAD","ref: refs/heads/main"]],"content":"","sig":"8d99262609796e477d1b55831da0c7cd16923b22191328149a59e3f151e52333a3678a2b9451bdfed7a3ff6fd93aec799fda23597ea48b0f3ea901d73549fc9e"}]
{"kind":30617,"id":"f66cc26f7693316db05cff6789333b9312468474a10b2511d439b51ecc0a1436","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764609631,"tags":[["d","noflow"],["r","aaf1722fde2f125862eb3353bb1810f0f653042f","euc"],["name","loom-protocol"],["description","Weaving Your Threads, Together."],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noflow.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noflow.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/noflow"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: loom-protocol"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"05f8381b9cd4dfa0ca564f618a1c39a05bea452867a2208f4f5bf6b51968589eb92728748e175b81c1934ae8275a3e2dd16dec663684abed64f5e7d994e622cb"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"4410562b0f8c86905b6a45223a7a5c646c09ffe3e0dfc8039532b9b93e4114ef","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764343273,"tags":[["d","nutsucker-2000"],["r","cd75fc0d6843cbba07c2731abe66b58ae2c81bcc","euc"],["name","nutsucker-2000"],["description","It sucks"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nutsucker-2000.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nutsucker-2000.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nutsucker-2000"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nutsucker-2000"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"a6a8bef7610f41c8776d614ca80ed3628a910686cd874cd9b31f50319999f9ebac59d1151b534f527541e1b36b1d28ca20279f46b29d80f12bf71f2a2315e61c"}]
{"kind":30618,"id":"cf80564e447768bfe0562f09e9ecacfb4f6c1f125f201b523ad81e56fd5fd8ac","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764343728,"tags":[["d","nutsucker-2000"],["refs/heads/main","a113781b8a32eaab64c44ebc68d3447f91fa4bec"],["HEAD","ref: refs/heads/main"]],"content":"","sig":"8d99262609796e477d1b55831da0c7cd16923b22191328149a59e3f151e52333a3678a2b9451bdfed7a3ff6fd93aec799fda23597ea48b0f3ea901d73549fc9e"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"de314f9dc64505815f3f7dd4e8179532657e8761eb9c38d7d91c565c8c520eee","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761217474,"tags":[["d","nostr-notary"],["HEAD","ref: refs/heads/main"],["refs/heads/main","1b4358d04c7e3e7ec7295b30462efffb7960b6e1"]],"content":"","sig":"dd2481285e185cf1bac1b5057b54813fee47415ad1dec80ef87b30dbdc929524c5eb86453dcb86146579fc4117eed92b64b2159099d143440b5d0f6dfb493248"}]
{"kind":30617,"id":"4410562b0f8c86905b6a45223a7a5c646c09ffe3e0dfc8039532b9b93e4114ef","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1764343273,"tags":[["d","nutsucker-2000"],["r","cd75fc0d6843cbba07c2731abe66b58ae2c81bcc","euc"],["name","nutsucker-2000"],["description","It sucks"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nutsucker-2000.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nutsucker-2000.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nutsucker-2000"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nutsucker-2000"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"a6a8bef7610f41c8776d614ca80ed3628a910686cd874cd9b31f50319999f9ebac59d1151b534f527541e1b36b1d28ca20279f46b29d80f12bf71f2a2315e61c"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"6198dc9563416e2b8841b927218d290ff3911ba24e671246b86c02ec808f6f1b","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761217442,"tags":[["d","nostr-notary"],["r","1b4358d04c7e3e7ec7295b30462efffb7960b6e1","euc"],["name","nostr-notary"],["description","Nostr hackday POC of notarizing nostr messages"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-notary.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-notary.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nostr-notary"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nostr-notary"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"f883912c9dc1278f60471e73ea8cc591e9745b5014a92b478c4a1f5fd75a5452745232718b74aa859b13c14e4e3d64fb333a55f16bdbeeff07dd17330ddc680c"}]
{"kind":30618,"id":"de314f9dc64505815f3f7dd4e8179532657e8761eb9c38d7d91c565c8c520eee","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761217474,"tags":[["d","nostr-notary"],["HEAD","ref: refs/heads/main"],["refs/heads/main","1b4358d04c7e3e7ec7295b30462efffb7960b6e1"]],"content":"","sig":"dd2481285e185cf1bac1b5057b54813fee47415ad1dec80ef87b30dbdc929524c5eb86453dcb86146579fc4117eed92b64b2159099d143440b5d0f6dfb493248"}
{"kind":30617,"id":"6198dc9563416e2b8841b927218d290ff3911ba24e671246b86c02ec808f6f1b","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761217442,"tags":[["d","nostr-notary"],["r","1b4358d04c7e3e7ec7295b30462efffb7960b6e1","euc"],["name","nostr-notary"],["description","Nostr hackday POC of notarizing nostr messages"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-notary.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-notary.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nostr-notary"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nostr-notary"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"f883912c9dc1278f60471e73ea8cc591e9745b5014a92b478c4a1f5fd75a5452745232718b74aa859b13c14e4e3d64fb333a55f16bdbeeff07dd17330ddc680c"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"145cb4fe81530a2bc1688cf20e9ff3ce2c4c29e66c01ddf231d9639dcb3dfdee","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761043127,"tags":[["d","fips-poc-sec"],["HEAD","ref: refs/heads/main"],["refs/heads/main","826b52ab29d9ca54e82c327f3aee0fee1d8214f7"]],"content":"","sig":"c45d6b67a32ee077166777421098ccfdd37545d4a111670661fd085a7512b05d4577eb2dd19f9b9745204a2e7d1882168340262a847e203586f3d439322bac51"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"20121d99e2cfde9f6ead27a57efe4cc4d12da4cc7aae6ca9721033d22815adfa","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761043073,"tags":[["d","fips-poc-sec"],["r","47adff995ba9c6780066932f3a659a6943b3e5a3","euc"],["name","fips-poc-sec"],["description","POC for routing with nostr pubkeys"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/fips-poc-sec.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/fips-poc-sec.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/fips-poc-sec"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: fips-poc-sec"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"363894244c14a152d7388a563d6c6ca5a0014f2070b819091b945c975488c678077ab1220dd4c2a0223a91d417dcba3954c5292772af5138f548144189989681"}]
{"kind":30618,"id":"145cb4fe81530a2bc1688cf20e9ff3ce2c4c29e66c01ddf231d9639dcb3dfdee","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761043127,"tags":[["d","fips-poc-sec"],["HEAD","ref: refs/heads/main"],["refs/heads/main","826b52ab29d9ca54e82c327f3aee0fee1d8214f7"]],"content":"","sig":"c45d6b67a32ee077166777421098ccfdd37545d4a111670661fd085a7512b05d4577eb2dd19f9b9745204a2e7d1882168340262a847e203586f3d439322bac51"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"d917b65b1c75d2da5cb8f4ec87eb36e83fb81105b230f1162ab567ad442c5a95","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1758995569,"tags":[["d","noports"],["refs/heads/main","68ad2c84c94271a93786846987d192c5d5df7a9c"],["HEAD","ref: refs/heads/main"]],"content":"","sig":"c267c2a234ecd3d03572832b9d49db94eb9e5abb7006ea61f307e866d65fd4a0cbb164b5e0052916da11f22746b2107da2e80fdec3dcacd31f359516148dd032"}]
{"kind":30617,"id":"20121d99e2cfde9f6ead27a57efe4cc4d12da4cc7aae6ca9721033d22815adfa","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1761043073,"tags":[["d","fips-poc-sec"],["r","47adff995ba9c6780066932f3a659a6943b3e5a3","euc"],["name","fips-poc-sec"],["description","POC for routing with nostr pubkeys"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/fips-poc-sec.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/fips-poc-sec.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/fips-poc-sec"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: fips-poc-sec"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"363894244c14a152d7388a563d6c6ca5a0014f2070b819091b945c975488c678077ab1220dd4c2a0223a91d417dcba3954c5292772af5138f548144189989681"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"57d949fcbf0c0486c55d4a4bf3afaf7f63b8509529735455573a232954c13924","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1758995549,"tags":[["d","noports"],["r","3f93f168394216d694e8b79e454f432ee30e18cd","euc"],["name","noports"],["description","Buy a public IP ingress controller for sats"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noports.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noports.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/noports"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: noports"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"4c105633c547fc54deaa17e6f3831749e21df6966be7ef8638dae5e9e8609cc80800f49e3debdc456a511fb028cc742b12386a97c3a5bd6d77183a2390e6a4b5"}]
{"kind":30618,"id":"d917b65b1c75d2da5cb8f4ec87eb36e83fb81105b230f1162ab567ad442c5a95","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1758995569,"tags":[["d","noports"],["refs/heads/main","68ad2c84c94271a93786846987d192c5d5df7a9c"],["HEAD","ref: refs/heads/main"]],"content":"","sig":"c267c2a234ecd3d03572832b9d49db94eb9e5abb7006ea61f307e866d65fd4a0cbb164b5e0052916da11f22746b2107da2e80fdec3dcacd31f359516148dd032"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"1db7af4ee4aba89c7ccf75c73b47efcf5f2490586617b44f2ee3e0d028d56c60","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1757667310,"tags":[["d","nostr-dns"],["r","4a114a7236aeb18a98b8e6e63b77c0a4b61fdbd8","euc"],["name","nostr-dns"],["description","resolves npub.nostr and npub.net (browser) using nip137 announcements"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-dns.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-dns.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nostr-dns"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nostr-dns"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"d682ca1e59207c97169f44db30f9861e7a219270e92f474cefda8e2f60eb2d3198d5cea0f2bfc7e9e87ae2eb76b14bb488e614e63977b4ceea839a198a376594"}]
{"kind":30617,"id":"57d949fcbf0c0486c55d4a4bf3afaf7f63b8509529735455573a232954c13924","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1758995549,"tags":[["d","noports"],["r","3f93f168394216d694e8b79e454f432ee30e18cd","euc"],["name","noports"],["description","Buy a public IP ingress controller for sats"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noports.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/noports.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/noports"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: noports"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"4c105633c547fc54deaa17e6f3831749e21df6966be7ef8638dae5e9e8609cc80800f49e3debdc456a511fb028cc742b12386a97c3a5bd6d77183a2390e6a4b5"}
{"kind":30617,"id":"1db7af4ee4aba89c7ccf75c73b47efcf5f2490586617b44f2ee3e0d028d56c60","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1757667310,"tags":[["d","nostr-dns"],["r","4a114a7236aeb18a98b8e6e63b77c0a4b61fdbd8","euc"],["name","nostr-dns"],["description","resolves npub.nostr and npub.net (browser) using nip137 announcements"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-dns.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/nostr-dns.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/nostr-dns"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: nostr-dns"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"d682ca1e59207c97169f44db30f9861e7a219270e92f474cefda8e2f60eb2d3198d5cea0f2bfc7e9e87ae2eb76b14bb488e614e63977b4ceea839a198a376594"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"cf7873cbc970269ce6e7abc9f7a670e8c2a842e0b1a49a29e5dbb11b8343f691","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1755695661,"tags":[["d","tollgate-module-basic-go"],["refs/heads/featrue-buy-from-upstream","ee7c8d4728913069fb839c4ffeea83e9e586d56f"],["refs/heads/bragging_mode","22a9d511b59f747cdc0edd7c8f1acad6b31bd1fc"],["refs/heads/build-with-docker","259e5312566c85c8a479efe48f13151c74001e04"],["refs/heads/just_janitor_2nd_try_connectivity_issues","1cac60857a1e3149d899521376d17cf0b2679b5a"],["refs/heads/multiple_mints_persistent_config","d104aadc86df74b8558c9a82617bd56ba432671e"],["refs/heads/feature/continous-automatic-verification","bb6261ecbf0bd1b9779a1d347d5e1a03c2c6b19a"],["refs/heads/feature-add-crowsnest","5252dcd5a649b403e69b23ec51a1037292940579"],["refs/heads/session-extension","2855546ace49123b37dfc026f95814671695b125"],["refs/heads/add_init.d_minimal_squashed","d7eb6edb29da1ad639a6722c02c4bf9376516fda"],["refs/heads/debug-old-version-for-os","9a2d00fe4611f411d9f9e4c1fdc286429c28197a"],["refs/heads/build-for-23","6a06fb5386598b76ecdd37a05fcb23f31379a70f"],["refs/heads/main","dbcb151103e91526e136475c037fe1304b8bfab9"],["refs/heads/feature/lightning-payments","dd289f42f5e140be5a4b43df98bbb7560e7f2407"],["refs/heads/janitor","8c7a9ab1664140841fb1bfeceb1686a7b225cd52"],["refs/heads/feature/pay-usptream-squashed","a5e0bce9ff50189516b2a99e87f84524119b49aa"],["refs/heads/add_tags","d4856c7b1443d64d2afcef2333f6bf79337fcbd3"],["refs/heads/add_init.d_minimal_squashed_rebased","a16c98414a98f005d3d114b5f5df5daba176a275"],["refs/heads/fix_ammount","b93494bbd88fceccebe6f54d1ec784ca2d017020"],["refs/heads/matrix","993f6054de8641a2c726d80fd298b5761bc97157"],["refs/heads/feature/discover-upstream","db834abe51405db680e759c2d5256493dcecd066"],["refs/heads/fix/install-config-nil","1a342af6f3096db1be4cfb76fbe2bcabbee953ed"],["refs/heads/feature/test-joel-frontend","2b0f0588eeea71832375c65da1f7cbaf81354128"],["refs/heads/add_gle750","bb341fd3e4fba6e88b6086f73f60299595b2eba2"],["refs/heads/feature-purchase-upstream","19979c05308f34cad0526d4db5a1ec945d7a2e93"],["refs/heads/banner_and_nodogsplash_restart","be3c9f0dd5cbddf14b2dd7511d42658e85a5e841"],["refs/heads/architecture-matrix","b608c8b45ae69700a938c8ac3c131b0ad0cca221"],["refs/heads/x1860","e746a1b96cb4a42b6c71863319f928ddf00d16bc"],["refs/heads/multiple_mints","301a8adba022c636787ac1ca35560d52ded41c81"],["refs/heads/bragging_mode_squashed","0cb147abfbd9e4a8651a74fa2f97ea2698f2c66d"],["refs/heads/refactor/merchant-testcases","15ce4da0ac3fd691e4474d816660167960524b1e"],["refs/heads/feature/joels-new-captive-portal","cfe3f914161ac3c6792630404a54c3532dc7008d"],["refs/heads/config-versioning","75b98a4b499a8ac723fb6f6de2075c0d7c6b1842"],["refs/heads/nobanner","f248e29d6533c43c930a162d6963635667d89e8b"],["refs/heads/feature/identity-config","6920c582d9963bb7f5b8a9bafc79038febec6dcc"],["refs/heads/just_janitor_2nd_try_squashed","e030f04f8aa722d421c1536d07e0b17919bfb6c4"],["refs/heads/nip94_os_event","9dd48d748558db8c90f83f75fb1093be87314cff"],["refs/heads/multi_arch","e5a7b51c8501b0fb7e8bbd03bda8efc82fb7408b"],["refs/heads/just_janitor","b6f8105a2f78174e57aab2ce0f22ed3230b14ea1"],["refs/heads/mt6000","646efc2b8acc9ec9b5464d5c8ce19605a6633478"],["refs/heads/fix/indentation-shell-scripts","6b77eda639be36907636d93b238d1e0d4b210324"],["refs/heads/feature/gateway-detection","b4f1e4384d1a010d9baa39b1442cb22d3c73458a"],["refs/heads/fix/losing-gateway-access","cd711463f17859e23e704cdcd2506181e2eefba8"],["refs/heads/add_init.d_minimal","450cff1cbf01e8a38cdeeab5df6bee18f0182772"],["refs/heads/add_init.d","eb0b4d80ecde4f663f1941c1102272f9cd2c41f6"],["refs/heads/fix_ecash_path","18ee765bafebc1b7ca02aaf9fbade34e01228d40"],["refs/heads/fix/identities.json","b90a787f192674be38e4b3f2bd20af2f7566d517"],["refs/heads/just_janitor_2nd_try","01e9d4d429923775bf14e397bdf6c75107d061c3"],["refs/heads/add_ar300m","3baffd810438cf9be3abecdf3324ed4fbc9d8c84"],["refs/heads/new-nip94","9cef6299b0e7b03d990e52daa61b776c5d8b79bc"],["refs/heads/nip94_os_event_squash","2f5f1ad99b8b0ad1ae5bafbedc43c7429c6018de"],["refs/heads/feature-add-crowsnest-squashed","87c269d018310643953587ae0d4c7fdbad7a76be"],["refs/heads/fix/identities.json-working","644ed0e463b99617d320b78a1fbe250af301e463"],["refs/heads/build_sdk","c33faa1910ff7651bc29e162d195635bb026cba4"],["refs/heads/fix/multiple_ssid_same_radio","efa9df48d69cab9583e9ecb03dbb52ce33b2cb70"],["HEAD","ref: refs/heads/main"],["refs/heads/restart-on-crash","a6b790f57a466ec2e712f15255cf6f73b577db5f"],["refs/heads/build_sdk_squashed","e40802471d83d4e64cc8a9bfcd1395c13aeb6e03"],["refs/heads/more_architectures","6d6654899c4b1f263358a9a5996fd3ac197075b6"],["refs/heads/janitor_works_on_laptop","c1285f0bba81f6a261b732199310b87543e9ff76"],["refs/heads/simplify/janitor-using-version-number-scheme","dd62cb5d6184dfad747f79480bfc39226a070ecb"]],"content":"","sig":"2ec86d8c1c2d0607f7617206f15e258a0f29f61c95766075c764c0e39f87c1fd80b33e150a32d0516256090d90e34d9b74a678cfee316f80bcf4787b6637f6f4"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"799bbf456fa36492f9b62c550b9ba3d7626c3e072a6b1d7dcd58034ba26af822","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1754475921,"tags":[["d","tollgate-module-basic-go"],["r","bdbd4457066573f1b1efe17be9dfee3c05569a76","euc"],["name","tollgate-module-basic-go"],["description","Basic TollGate functionality"],["clone","https://github.com/OpenTollGate/tollgate-module-basic-go.git","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-module-basic-go.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-module-basic-go.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-module-basic-go"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://nos.lol/","wss://bitcoiner.social/","wss://relay.damus.io/","wss://relay.primal.net/"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","3539591f412ee5a76aca22d727e95420e8596c2d0d2527db504d21b539e6cf50","c3e23eb5e3d00f18b2f4f588d8cdbc548648be761bdd90812186df4603d7caa9"],["alt","git repository: tollgate-module-basic-go"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"4e1039f5b49c710d58469f13ccadd538892d8266ad2bb3b6f2b297e9fd110ef15170d9ffd4a60f8e05d8c285b53500ff39b90e514b594cd20fbf0627af881d0f"}]
{"kind":30618,"id":"cf7873cbc970269ce6e7abc9f7a670e8c2a842e0b1a49a29e5dbb11b8343f691","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1755695661,"tags":[["d","tollgate-module-basic-go"],["refs/heads/featrue-buy-from-upstream","ee7c8d4728913069fb839c4ffeea83e9e586d56f"],["refs/heads/bragging_mode","22a9d511b59f747cdc0edd7c8f1acad6b31bd1fc"],["refs/heads/build-with-docker","259e5312566c85c8a479efe48f13151c74001e04"],["refs/heads/just_janitor_2nd_try_connectivity_issues","1cac60857a1e3149d899521376d17cf0b2679b5a"],["refs/heads/multiple_mints_persistent_config","d104aadc86df74b8558c9a82617bd56ba432671e"],["refs/heads/feature/continous-automatic-verification","bb6261ecbf0bd1b9779a1d347d5e1a03c2c6b19a"],["refs/heads/feature-add-crowsnest","5252dcd5a649b403e69b23ec51a1037292940579"],["refs/heads/session-extension","2855546ace49123b37dfc026f95814671695b125"],["refs/heads/add_init.d_minimal_squashed","d7eb6edb29da1ad639a6722c02c4bf9376516fda"],["refs/heads/debug-old-version-for-os","9a2d00fe4611f411d9f9e4c1fdc286429c28197a"],["refs/heads/build-for-23","6a06fb5386598b76ecdd37a05fcb23f31379a70f"],["refs/heads/main","dbcb151103e91526e136475c037fe1304b8bfab9"],["refs/heads/feature/lightning-payments","dd289f42f5e140be5a4b43df98bbb7560e7f2407"],["refs/heads/janitor","8c7a9ab1664140841fb1bfeceb1686a7b225cd52"],["refs/heads/feature/pay-usptream-squashed","a5e0bce9ff50189516b2a99e87f84524119b49aa"],["refs/heads/add_tags","d4856c7b1443d64d2afcef2333f6bf79337fcbd3"],["refs/heads/add_init.d_minimal_squashed_rebased","a16c98414a98f005d3d114b5f5df5daba176a275"],["refs/heads/fix_ammount","b93494bbd88fceccebe6f54d1ec784ca2d017020"],["refs/heads/matrix","993f6054de8641a2c726d80fd298b5761bc97157"],["refs/heads/feature/discover-upstream","db834abe51405db680e759c2d5256493dcecd066"],["refs/heads/fix/install-config-nil","1a342af6f3096db1be4cfb76fbe2bcabbee953ed"],["refs/heads/feature/test-joel-frontend","2b0f0588eeea71832375c65da1f7cbaf81354128"],["refs/heads/add_gle750","bb341fd3e4fba6e88b6086f73f60299595b2eba2"],["refs/heads/feature-purchase-upstream","19979c05308f34cad0526d4db5a1ec945d7a2e93"],["refs/heads/banner_and_nodogsplash_restart","be3c9f0dd5cbddf14b2dd7511d42658e85a5e841"],["refs/heads/architecture-matrix","b608c8b45ae69700a938c8ac3c131b0ad0cca221"],["refs/heads/x1860","e746a1b96cb4a42b6c71863319f928ddf00d16bc"],["refs/heads/multiple_mints","301a8adba022c636787ac1ca35560d52ded41c81"],["refs/heads/bragging_mode_squashed","0cb147abfbd9e4a8651a74fa2f97ea2698f2c66d"],["refs/heads/refactor/merchant-testcases","15ce4da0ac3fd691e4474d816660167960524b1e"],["refs/heads/feature/joels-new-captive-portal","cfe3f914161ac3c6792630404a54c3532dc7008d"],["refs/heads/config-versioning","75b98a4b499a8ac723fb6f6de2075c0d7c6b1842"],["refs/heads/nobanner","f248e29d6533c43c930a162d6963635667d89e8b"],["refs/heads/feature/identity-config","6920c582d9963bb7f5b8a9bafc79038febec6dcc"],["refs/heads/just_janitor_2nd_try_squashed","e030f04f8aa722d421c1536d07e0b17919bfb6c4"],["refs/heads/nip94_os_event","9dd48d748558db8c90f83f75fb1093be87314cff"],["refs/heads/multi_arch","e5a7b51c8501b0fb7e8bbd03bda8efc82fb7408b"],["refs/heads/just_janitor","b6f8105a2f78174e57aab2ce0f22ed3230b14ea1"],["refs/heads/mt6000","646efc2b8acc9ec9b5464d5c8ce19605a6633478"],["refs/heads/fix/indentation-shell-scripts","6b77eda639be36907636d93b238d1e0d4b210324"],["refs/heads/feature/gateway-detection","b4f1e4384d1a010d9baa39b1442cb22d3c73458a"],["refs/heads/fix/losing-gateway-access","cd711463f17859e23e704cdcd2506181e2eefba8"],["refs/heads/add_init.d_minimal","450cff1cbf01e8a38cdeeab5df6bee18f0182772"],["refs/heads/add_init.d","eb0b4d80ecde4f663f1941c1102272f9cd2c41f6"],["refs/heads/fix_ecash_path","18ee765bafebc1b7ca02aaf9fbade34e01228d40"],["refs/heads/fix/identities.json","b90a787f192674be38e4b3f2bd20af2f7566d517"],["refs/heads/just_janitor_2nd_try","01e9d4d429923775bf14e397bdf6c75107d061c3"],["refs/heads/add_ar300m","3baffd810438cf9be3abecdf3324ed4fbc9d8c84"],["refs/heads/new-nip94","9cef6299b0e7b03d990e52daa61b776c5d8b79bc"],["refs/heads/nip94_os_event_squash","2f5f1ad99b8b0ad1ae5bafbedc43c7429c6018de"],["refs/heads/feature-add-crowsnest-squashed","87c269d018310643953587ae0d4c7fdbad7a76be"],["refs/heads/fix/identities.json-working","644ed0e463b99617d320b78a1fbe250af301e463"],["refs/heads/build_sdk","c33faa1910ff7651bc29e162d195635bb026cba4"],["refs/heads/fix/multiple_ssid_same_radio","efa9df48d69cab9583e9ecb03dbb52ce33b2cb70"],["HEAD","ref: refs/heads/main"],["refs/heads/restart-on-crash","a6b790f57a466ec2e712f15255cf6f73b577db5f"],["refs/heads/build_sdk_squashed","e40802471d83d4e64cc8a9bfcd1395c13aeb6e03"],["refs/heads/more_architectures","6d6654899c4b1f263358a9a5996fd3ac197075b6"],["refs/heads/janitor_works_on_laptop","c1285f0bba81f6a261b732199310b87543e9ff76"],["refs/heads/simplify/janitor-using-version-number-scheme","dd62cb5d6184dfad747f79480bfc39226a070ecb"]],"content":"","sig":"2ec86d8c1c2d0607f7617206f15e258a0f29f61c95766075c764c0e39f87c1fd80b33e150a32d0516256090d90e34d9b74a678cfee316f80bcf4787b6637f6f4"}
{"kind":30617,"id":"799bbf456fa36492f9b62c550b9ba3d7626c3e072a6b1d7dcd58034ba26af822","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1754475921,"tags":[["d","tollgate-module-basic-go"],["r","bdbd4457066573f1b1efe17be9dfee3c05569a76","euc"],["name","tollgate-module-basic-go"],["description","Basic TollGate functionality"],["clone","https://github.com/OpenTollGate/tollgate-module-basic-go.git","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-module-basic-go.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-module-basic-go.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-module-basic-go"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://nos.lol/","wss://bitcoiner.social/","wss://relay.damus.io/","wss://relay.primal.net/"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","3539591f412ee5a76aca22d727e95420e8596c2d0d2527db504d21b539e6cf50","c3e23eb5e3d00f18b2f4f588d8cdbc548648be761bdd90812186df4603d7caa9"],["alt","git repository: tollgate-module-basic-go"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"4e1039f5b49c710d58469f13ccadd538892d8266ad2bb3b6f2b297e9fd110ef15170d9ffd4a60f8e05d8c285b53500ff39b90e514b594cd20fbf0627af881d0f"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"14f05c0749b1600824452c0a6c2f9f01fc6a2b1729e8616ffcb5f9c10f00b091","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1753540249,"tags":[["d","tollgate-management-portal-site"],["refs/heads/main","6f734531ea1586b8e3bd03b0a7f0895ca66a2cf0"]],"content":"","sig":"d1a92f39110705465d7f8eba036141fadca34d0bb301a3d80027da3fc0cf2863a5a6e51b7261c8a5b104618d594e1732ab0c261004973df0f3cbd9e638dcf6d3"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"07103bed0b46b1d2652bb9cd0e8870c930d2280117d11f37f27e10cf6f60ec6e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1753540229,"tags":[["d","tollgate-management-portal-site"],["r","6f734531ea1586b8e3bd03b0a7f0895ca66a2cf0","euc"],["name","tollgate-management-portal-site"],["description","TollGate Portal for managing your device and user session info"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-management-portal-site.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-management-portal-site.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-management-portal-site"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","c3e23eb5e3d00f18b2f4f588d8cdbc548648be761bdd90812186df4603d7caa9","53a91e3a64d1f658e983ac1e4f9e0c697f8f33e01d8debe439f4c1a92113f592"],["alt","git repository: tollgate-management-portal-site"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"f39b6985d5216e3c6d13e7a1bc9b83723bc978bde421068f326242c231fa9aebffda2a56ce145d102305d1432cf23b9408b196dabcd25e19c54f243d2936c95f"}]
{"kind":30617,"id":"07103bed0b46b1d2652bb9cd0e8870c930d2280117d11f37f27e10cf6f60ec6e","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1753540229,"tags":[["d","tollgate-management-portal-site"],["r","6f734531ea1586b8e3bd03b0a7f0895ca66a2cf0","euc"],["name","tollgate-management-portal-site"],["description","TollGate Portal for managing your device and user session info"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-management-portal-site.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-management-portal-site.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-management-portal-site"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","c3e23eb5e3d00f18b2f4f588d8cdbc548648be761bdd90812186df4603d7caa9","53a91e3a64d1f658e983ac1e4f9e0c697f8f33e01d8debe439f4c1a92113f592"],["alt","git repository: tollgate-management-portal-site"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"f39b6985d5216e3c6d13e7a1bc9b83723bc978bde421068f326242c231fa9aebffda2a56ce145d102305d1432cf23b9408b196dabcd25e19c54f243d2936c95f"}
{"kind":30618,"id":"14f05c0749b1600824452c0a6c2f9f01fc6a2b1729e8616ffcb5f9c10f00b091","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1753540249,"tags":[["d","tollgate-management-portal-site"],["refs/heads/main","6f734531ea1586b8e3bd03b0a7f0895ca66a2cf0"]],"content":"","sig":"d1a92f39110705465d7f8eba036141fadca34d0bb301a3d80027da3fc0cf2863a5a6e51b7261c8a5b104618d594e1732ab0c261004973df0f3cbd9e638dcf6d3"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"1eeeea19e0d72261658ca18334c8a35e50215fd64c010354de909cff477eedf5","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1751360305,"tags":[["d","ngit-repo-explorer"],["refs/heads/commit-history","3f0dc54c664795583213118c27c218f52b738428"],["refs/heads/main","d8e6ada5051db1f205e580ce1d5ac12cbe0f0bc2"],["refs/heads/master","e00e8101a8310070cc221ff15f0818d88cc68b1d"],["HEAD","ref: refs/heads/main"],["refs/heads/next","d8e6ada5051db1f205e580ce1d5ac12cbe0f0bc2"]],"content":"","sig":"9b6ec9bb74e8249aa8db54cae02caae9270da1a01490b9b2753386dc621e0cfdcb0c018b532b6bf4b7bb4d50ee3a4cc3310b5505a4cc42999a1cf7e4a73c06f5"}]
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"13db013898f0c2e63d523213fd5f81f4201a90de183928580ac7d431524cd57c","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750455475,"tags":[["d","ngit-repo-explorer"],["r","b540fe617eadbb92800999a4e57a91ad8cabd2f0","euc"],["name","treegaze"],["description","Explore and browse files of Nostr-native Git repostiories"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/ngit-repo-explorer.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/ngit-repo-explorer.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/ngit-repo-explorer"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: treegaze"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"85a496e2b8dd1495de814f2f13a9468a2b4d8403122b98f6189b975f5433ef3ee181b81c7605cdf98c0329b72807c512f9040f7f6360eaf97b2fca025f2ae506"}]
{"kind":30618,"id":"1eeeea19e0d72261658ca18334c8a35e50215fd64c010354de909cff477eedf5","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1751360305,"tags":[["d","ngit-repo-explorer"],["refs/heads/commit-history","3f0dc54c664795583213118c27c218f52b738428"],["refs/heads/main","d8e6ada5051db1f205e580ce1d5ac12cbe0f0bc2"],["refs/heads/master","e00e8101a8310070cc221ff15f0818d88cc68b1d"],["HEAD","ref: refs/heads/main"],["refs/heads/next","d8e6ada5051db1f205e580ce1d5ac12cbe0f0bc2"]],"content":"","sig":"9b6ec9bb74e8249aa8db54cae02caae9270da1a01490b9b2753386dc621e0cfdcb0c018b532b6bf4b7bb4d50ee3a4cc3310b5505a4cc42999a1cf7e4a73c06f5"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30617,"id":"1acc9786fe08b764ff2c4b80c97904ce9431d9fc749ec2798f2c4baad68a5650","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750247391,"tags":[["d","tollgate-desktop-client"],["r","7fbea28cbc3c00500630284a1572ba43abd5d950","euc"],["name","tollgate-desktop-client"],["description","Desktop client for auto-purchasing internet access"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-desktop-client.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-desktop-client.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-desktop-client"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: tollgate-desktop-client"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"14a06df253303372b1c8347181e2b1197827df9519643e068896ab51b85267c72ab98e03bc6f7bcee879163854f4ef1b40a177182c1c71831b8d6edaf8d96c36"}]
{"kind":30617,"id":"13db013898f0c2e63d523213fd5f81f4201a90de183928580ac7d431524cd57c","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750455475,"tags":[["d","ngit-repo-explorer"],["r","b540fe617eadbb92800999a4e57a91ad8cabd2f0","euc"],["name","treegaze"],["description","Explore and browse files of Nostr-native Git repostiories"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/ngit-repo-explorer.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/ngit-repo-explorer.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/ngit-repo-explorer"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: treegaze"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"85a496e2b8dd1495de814f2f13a9468a2b4d8403122b98f6189b975f5433ef3ee181b81c7605cdf98c0329b72807c512f9040f7f6360eaf97b2fca025f2ae506"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EVENT","1:nak-req",{"kind":30618,"id":"7c0e7d178b51c37d0fe886271b41b98170358cf5338fe3f161a89434e4a397ff","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750242566,"tags":[["d","tollgate-desktop-client"],["refs/heads/main","f30c7c8e67451582c3cd7b1d361caaa9f90afec0"]],"content":"","sig":"fed7b1967f9fa37f19f0ada206a8ea7bbb3e89ce1fb3cbbd0de897c343e6ce9a5c951d0c05923c4bfa3942127761d1d37a573d1ff9a0ba33932e107292b90636"}]
{"kind":30617,"id":"1acc9786fe08b764ff2c4b80c97904ce9431d9fc749ec2798f2c4baad68a5650","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750247391,"tags":[["d","tollgate-desktop-client"],["r","7fbea28cbc3c00500630284a1572ba43abd5d950","euc"],["name","tollgate-desktop-client"],["description","Desktop client for auto-purchasing internet access"],["clone","https://relay.ngit.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-desktop-client.git","https://gitnostr.com/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/tollgate-desktop-client.git"],["web","https://gitworkshop.dev/npub1hw6amg8p24ne08c9gdq8hhpqx0t0pwanpae9z25crn7m9uy7yarse465gr/relay.ngit.dev/tollgate-desktop-client"],["relays","wss://relay.ngit.dev","wss://gitnostr.com","wss://relay.damus.io","wss://nos.lol","wss://relay.nostr.band"],["maintainers","bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747"],["alt","git repository: tollgate-desktop-client"],["blossoms","https://relay.ngit.dev","https://gitnostr.com"]],"content":"","sig":"14a06df253303372b1c8347181e2b1197827df9519643e068896ab51b85267c72ab98e03bc6f7bcee879163854f4ef1b40a177182c1c71831b8d6edaf8d96c36"}
[nl][debug] 2025/12/05 22:00:53 {wss://gitnostr.com} received ["EOSE","1:nak-req"]
{"kind":30618,"id":"7c0e7d178b51c37d0fe886271b41b98170358cf5338fe3f161a89434e4a397ff","pubkey":"bbb5dda0e15567979f0543407bdc2033d6f0bbb30f72512a981cfdb2f09e2747","created_at":1750242566,"tags":[["d","tollgate-desktop-client"],["refs/heads/main","f30c7c8e67451582c3cd7b1d361caaa9f90afec0"]],"content":"","sig":"fed7b1967f9fa37f19f0ada206a8ea7bbb3e89ce1fb3cbbd0de897c343e6ce9a5c951d0c05923c4bfa3942127761d1d37a573d1ff9a0ba33932e107292b90636"}
</code>
</pre>
</details>

### from a file with events get only those that have kind 1111 and were created by a given pubkey
```shell
~> cat all.jsonl | nak filter -k 1111 -a 117673e191b10fe1aedf1736ee74de4cffd4c132ca701960b70a5abad5870faa > filtered.jsonl
```

### use negentropy (nip77) to only fetch the ids for a given query
```shell
~> nak req --ids-only -k 1111 -a npub1vyrx2prp0mne8pczrcvv38ahn5wahsl8hlceeu3f3aqyvmu8zh5s7kfy55 relay.damus.io
```

### manage nip34/grasp git repositories
```shell
~> nak git clone
~> nak git init
~> nak git sync
~> nak git fetch
~> nak git pull
~> nak git push
```

[demo screencast](https://njump.me/nevent1qvzqqqqqqypzqwlsccluhy6xxsr6l9a9uhhxf75g85g8a709tprjcn4e42h053vaqqswfth72qet7p4tdgvd92wpq4zcerseu3ecwqkac622xad5wqln6jsta5zpv).

### generate a new random key and print its associated public key at the same time
```shell
~> nak key generate | pee 'nak encode nsec' 'nak key public | nak encode npub'
1a851afaa70a26faa82c5b4422ce967c07e278efc56a1413b9719b662f86551a
8031621a54b2502f5bd4dbb87c971c0a69675d252a64d69e22224f3aee6dd2b2
```
