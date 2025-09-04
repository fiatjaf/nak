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
...
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

```shell
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

### from a file with events get only those that have kind 1111 and were created by a given pubkey
```shell
~> cat all.jsonl | nak filter -k 1111 -a 117673e191b10fe1aedf1736ee74de4cffd4c132ca701960b70a5abad5870faa > filtered.jsonl
```
