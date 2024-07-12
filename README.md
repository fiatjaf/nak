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

### fetch all quoted events by a given pubkey in their last 100 notes
```shell
~> nak req -l 100 -k 1 -a 2edbcea694d164629854a52583458fd6d965b161e3c48b57d3aff01940558884 relay.damus.io | jq -r '.content | match("nostr:((note1|nevent1)[a-z0-9]+)";"g") | .captures[0].string' | nak decode | jq -cr '{ids: [.id]}' | nak req relay.damus.io
connecting to relay.damus.io...
ok.
{"kind":1,"id":"dad32411fea62fda6ae057e97c73402f2031913388a721e059728a0efee5f0dd","pubkey":"5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","created_at":1709057416,"tags":[["p","ad9d42203fd2480ea2e5c4c64593a027708aebe2b02aa60bd7b1d666daa5b08d"],["p","5ea721dd7828229a39a372477090208db30a6c2d357951b8ae504d2ecf86c06c"]],"content":"Fridays Edition of nostr:npub14kw5ygpl6fyqagh9cnrytyaqyacg46lzkq42vz7hk8txdk49kzxs04j7y0 will feature nostr:npub1t6njrhtc9q3f5wdrwfrhpypq3kes5mpdx4u4rw9w2pxjanuxcpkqveagv3 \n\nWe will be diving into Bitcoin, Content Creating, Music and the future of V4V. \n\nNostr Nest 2.0 🤘🏻\nSet your BlockClocks for 4:30EST","sig":"69cc403982e6c5fe996d545d6057c581a46be97ab79d818c1bc01e84e9f11a64a275d8834a4063a59fa135f7f116e38c51125173d5ce88671a4ddc2f656e01e4"}
{"kind":1,"id":"a681e9ca594dc455018be0a1c895576a8264956aee3e4fc01b872aa6df580632","pubkey":"5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","created_at":1708456729,"tags":[["p","ad9d42203fd2480ea2e5c4c64593a027708aebe2b02aa60bd7b1d666daa5b08d"],["t","plebchain"],["p","4ce6abbd68dab6e9fdf6e8e9912a8e12f9b539e078c634c55a9bff2994a514dd"],["p","b83a28b7e4e5d20bd960c5faeb6625f95529166b8bdb045d42634a2f35919450"],["p","5a9c48c8f4782351135dd89c5d8930feb59cb70652ffd37d9167bf922f2d1069"],["p","f8e6c64342f1e052480630e27e1016dce35fc3a614e60434fef4aa2503328ca9"],["p","2edbcea694d164629854a52583458fd6d965b161e3c48b57d3aff01940558884"],["r","https://nostrnests.com/plebchainradio"]],"content":"This weeks edition of nostr:npub14kw5ygpl6fyqagh9cnrytyaqyacg46lzkq42vz7hk8txdk49kzxs04j7y0 will feature the wildly talented #Plebchain Legend nostr:npub1fnn2h0tgm2mwnl0kar5ez25wztum2w0q0rrrf326n0ljn999znwsqf4xnx \n\nWe will be discussing building his local community in Tanzania, his worldly success in V4V music and all things Nostr and Bitcoin. \n\nFilling in for nostr:npub1hqaz3dlyuhfqhktqchawke39l92jj9nt30dsgh2zvd9z7dv3j3gqpkt56s this week will be nostr:npub1t2wy3j850q34zy6amzw9mzfsl66eedcx2tlaxlv3v7leytedzp5szs8c2u \u0026 nostr:npub1lrnvvs6z78s9yjqxxr38uyqkmn34lsaxznnqgd877j4z2qej3j5s09qnw5 from nostr:npub19mduaf5569jx9xz555jcx3v06mvktvtpu0zgk47n4lcpjsz43zzqhj6vzk \n\nSet your Blockclocks for this Friday 4PM EST! \nhttps://nostrnests.com/plebchainradio","sig":"b4528c7a248bf04ab9fcd0ce8033fdc9656b0e92dccf5f3a6b8cd7ad66cf074619100c7d192ae9a87745bc5445f6fe36221c1fd5820d5038bbcae2aedb5090d8"}
{"kind":1,"id":"cc396bbc9e01910e56ef169916c39197d468b65e80c42aaa0a874a32500039c4","pubkey":"b9d02cb8fddeb191701ec0648e37ed1f6afba263e0060fc06099a62851d25e04","created_at":1708210291,"tags":[["imeta","url https://image.nostr.build/185e4c10dc2e46dacf6ad38fbc4319f52860fc3f96efb433be55bdb91fc225b8.jpg","blurhash eUGR9vR*4pt6OqK0WVspWWoL0fso%eWBwM+~aySxs:S2tQW;VtWVW;","dim 4032x3024"],["p","f133b246f07633fde1a894133ac270ab8750502b64a9779c0bac3c9228198dda"],["p","5ea721dd7828229a39a372477090208db30a6c2d357951b8ae504d2ecf86c06c"],["t","cultureshock"],["r","https://image.nostr.build/185e4c10dc2e46dacf6ad38fbc4319f52860fc3f96efb433be55bdb91fc225b8.jpg"]],"content":"10 minutes.\n\nFireside chat between nostr:npub17yemy3hswcelmcdgjsfn4sns4wr4q5ptvj5h08qt4s7fy2qe3hdqsczs99 and nostr:npub1t6njrhtc9q3f5wdrwfrhpypq3kes5mpdx4u4rw9w2pxjanuxcpkqveagv3 followed by live set to close down #CultureShock.\n\nStreaming V4V at tunestr.io https://image.nostr.build/185e4c10dc2e46dacf6ad38fbc4319f52860fc3f96efb433be55bdb91fc225b8.jpg ","sig":"b0464529b2d2de2b4df911d47dbfe4aa31ac8f2db285b1a5930941cf1877425c14d6901d1be1bfaa13cd3d981d5e9a722debfc47c4f087d95da628a7035437ec"}
{"kind":1,"id":"f24aed86b493f266952ed35d4724582946cdf73985581f9986641f81fad6b73d","pubkey":"fa984bd7dbb282f07e16e7ae87b26a2a7b9b90b7246a44771f0cf5ae58018f52","created_at":1707912005,"tags":[["p","fa984bd7dbb282f07e16e7ae87b26a2a7b9b90b7246a44771f0cf5ae58018f52","wss://140.f7z.io/","mention"]],"content":"Releasing: Whynotstr 0.0.0 😂\n\nCollaborative editing, the left-side-of-the-curve approach\n\nTry it out: https://collab-lemon.vercel.app/\n\nObviously open source: https://github.com/pablof7z/collab\n\nnostr:nevent1qvzqqqqqqypzp75cf0tahv5z7plpdeaws7ex52nmnwgtwfr2g3m37r844evqrr6jqy88wumn8ghj7mn0wvhxcmmv9uq35amnwvaz7tms09exzmtfvshxv6tpw34xze3wvdhk6tcqyznqm6guz9k38dpqtc84s7jflec7m7wpzvtx2xjkjkm2xm89ucy2jhdcnur","sig":"6f562d733e50f5934dcf359a4f16dece1734302c0cc3a793ee2f08007ccb4ade3591373a633538617611f327feb7534ad4d11a8475163c7f734a01c63e52b79f"}
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
~> nak key generate                                                                              18:59
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

### sign an event using a remote NIP-46 bunker
```shell
~> nak event --connect 'bunker://a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208?relay=wss%3A%2F%2Frelay.damus.io&relay=wss%3A%2F%2Frelay.nsecbunker.com&relay=wss%3A%2F%2Fnos.lol&secret=TWfGbjQCLxUf' -c 'hello from bunker'
```

### sign an event using a NIP-49 encrypted key
```shell
~> nak event --sec ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls -c 'hello from encrypted key'
type the password to decrypt your secret key: **********
{"kind":1,"id":"8aa5c931fb1da507f14801de6a1814b7f0baae984dc502b9889f347f5aa3cc4e","pubkey":"985d66d2644dfa7676e26046914470d66ebc7fa783a3f57f139fde32d0d631d7","created_at":1720822280,"tags":[],"content":"hello from encrypted key","sig":"9d1c9e56e87f787cc5b6191ec47690ce59fa4bef105b56297484253953e18fb930f6683f007e84a9ce9dc9a25b20c191c510629156dcd24bd16e15d302d20944"}
```

### talk to a relay's NIP-86 management API
```shell
nak relay allowpubkey --sec ncryptsec1qggx54cg270zy9y8krwmfz29jyypsuxken2fkk99gr52qhje968n6mwkrfstqaqhq9eq94pnzl4nff437l4lp4ur2cs4f9um8738s35l2esx2tas48thtfhrk5kq94pf9j2tpk54yuermra0xu6hl5ls --pubkey a9e0f110f636f3191644110c19a33448daf09d7cda9708a769e91b7e91340208 pyramid.fiatjaf.com
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

## contributing to this repository

Use NIP-34 to send your patches to `naddr1qqpkucttqy28wumn8ghj7un9d3shjtnwdaehgu3wvfnsz9nhwden5te0wfjkccte9ehx7um5wghxyctwvsq3gamnwvaz7tmjv4kxz7fwv3sk6atn9e5k7q3q80cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsxpqqqpmej2wctpn`.
