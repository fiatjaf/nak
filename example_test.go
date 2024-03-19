package main

import "os"

func ExampleEventBasic() {
	app.Run([]string{"nak", "event", "--ts", "1699485669"})
	// Output:
	// {"id":"36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"68e71a192e8abcf8582a222434ac823ecc50607450ebe8cc4c145eb047794cc382dc3f888ce879d2f404f5ba6085a47601360a0fa2dd4b50d317bd0c6197c2c2"}
}

// (for some reason there can only be one test dealing with stdin in the suite otherwise it halts)
func ExampleEventParsingFromStdin() {
	prevStdin := os.Stdin
	defer func() { os.Stdin = prevStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.WriteString("{\"content\":\"hello world\"}\n{\"content\":\"hello sun\"}\n")
	app.Run([]string{"nak", "event", "-t", "t=spam", "--ts", "1699485669"})
	// Output:
	// {"id":"bda134f9077c11973afe6aa5a1cc6f5bcea01c40d318b8f91dcb8e50507cfa52","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[["t","spam"]],"content":"hello world","sig":"7552454bb8e7944230142634e3e34ac7468bad9b21ed6909da572c611018dff1d14d0792e98b5806f6330edc51e09efa6d0b66a9694dc34606c70f4e580e7493"}
	// {"id":"879c36ec73acca288825b53585389581d3836e7f0fe4d46e5eba237ca56d6af5","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[["t","spam"]],"content":"hello sun","sig":"6c7e6b13ebdf931d26acfdd00bec2ec1140ddaf8d1ed61453543a14e729a460fe36c40c488ccb194a0e1ab9511cb6c36741485f501bdb93c39ca4c51bc59cbd4"}
}

func ExampleEventComplex() {
	app.Run([]string{"nak", "event", "--ts", "1699485669", "-k", "11", "-c", "skjdbaskd", "--sec", "17", "-t", "t=spam", "-e", "36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c", "-t", "r=https://abc.def?name=foobar;nothing"})
	// Output:
	// {"id":"19aba166dcf354bf5ef64f4afe69ada1eb851495001ee05e07d393ee8c8ea179","pubkey":"2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f","created_at":1699485669,"kind":11,"tags":[["t","spam"],["r","https://abc.def?name=foobar","nothing"],["e","36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c"]],"content":"skjdbaskd","sig":"cf452def4a68341c897c3fc96fa34dc6895a5b8cc266d4c041bcdf758ec992ec5adb8b0179e98552aaaf9450526a26d7e62e413b15b1c57e0cfc8db6b29215d7"}
}

func ExampleEncode() {
	app.Run([]string{"nak", "encode", "npub", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822"})
	app.Run([]string{"nak", "encode", "nprofile", "-r", "wss://example.com", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822"})
	app.Run([]string{"nak", "encode", "nprofile", "-r", "wss://example.com", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822", "a5592173975ded9f836a9572ea8b11a7e16ceb66464d66d50b27163f7f039d2c"})
	// npub156n8a7wuhwk9tgrzjh8gwzc8q2dlekedec5djk0js9d3d7qhnq3qjpdq28
	// nprofile1qqs2dfn7l8wthtz45p3ftn58pvrs9xlumvkuu2xet8egzkcklqtesgspz9mhxue69uhk27rpd4cxcefwvdhk6fl5jug
	// nprofile1qqs2dfn7l8wthtz45p3ftn58pvrs9xlumvkuu2xet8egzkcklqtesgspz9mhxue69uhk27rpd4cxcefwvdhk6fl5jug
	// nprofile1qqs22kfpwwt4mmvlsd4f2uh23vg60ctvadnyvntx659jw93l0upe6tqpz9mhxue69uhk27rpd4cxcefwvdhk64h265a
}

func ExampleDecode() {
	app.Run([]string{"nak", "decode", "naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu", "nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8"})
	// Output:
	// {
	//   "pubkey": "5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e",
	//   "kind": 31923,
	//   "identifier": "4cd6cfe7",
	//   "relays": [
	//     "wss://nos.lol"
	//   ]
	// }
	// {
	//   "id": "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5",
	//   "relays": [
	//     "wss://pyramid.fiatjaf.com/",
	//     "wss://relay.westernbtc.com/",
	//     "wss://relay.snort.social/",
	//     "wss://atlas.nostr.land/"
	//   ]
	// }
}

func ExampleReq() {
	app.Run([]string{"nak", "req", "-k", "1", "-l", "18", "-a", "2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f", "-e", "aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"})
	// Output:
	// ["REQ","nak",{"kinds":[1],"authors":["2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f"],"limit":18,"#e":["aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"]}]
}

func ExampleReqIdFromRelay() {
	app.Run([]string{"nak", "req", "-i", "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5", "wss://nostr.wine"})
	// Output:
	// {"id":"3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5","pubkey":"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d","created_at":1710759386,"kind":1,"tags":[],"content":"Nostr was coopted by our the corporate overlords. It is now featured in https://www.iana.org/assignments/well-known-uris/well-known-uris.xhtml.","sig":"faaec167cca4de50b562b7702e8854e2023f0ccd5f36d1b95b6eac20d352206342d6987e9516d283068c768e94dbe8858e2990c3e05405e707fb6fb771ef92f9"}
}

func ExampleMultipleFetch() {
	app.Run([]string{"nak", "fetch", "naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu", "nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8"})
	// Output:
	// {"id":"9ae5014573fc75ced00b343868d2cd9343ebcbbae50591c6fa8ae1cd99568f05","pubkey":"5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","created_at":1707764605,"kind":31923,"tags":[["d","4cd6cfe7"],["name","Nostr PHX Presents Culture Shock"],["description","Nostr PHX presents Culture Shock the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, food, drinks, conversation, vendors and best of all, a live convert which will stream globally for the world to zap. "],["start","1708185600"],["end","1708228800"],["start_tzid","America/Phoenix"],["p","5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","","host"],["location","Hello Merch, 850 W Lincoln St, Phoenix, AZ 85007, USA","Hello Merch","850 W Lincoln St, Phoenix, AZ 85007, USA"],["address","Hello Merch, 850 W Lincoln St, Phoenix, AZ 85007, USA","Hello Merch","850 W Lincoln St, Phoenix, AZ 85007, USA"],["g","9tbq1rzn"],["image","https://flockstr.s3.amazonaws.com/event/15vSaiscDhVH1KBXhA0i8"],["about","Nostr PHX presents Culture Shock : the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, conversation, food and goods which will be capped off with a live concert streamed globally for the world to boost \u0026 zap. \n\nWe strive to source local vendors, local artists, local partnerships. Please reach out to us if you are interested in participating in this historic event. "],["calendar","31924:5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e:1f238c94"]],"content":"Nostr PHX presents Culture Shock : the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, conversation, food and goods which will be capped off with a live concert streamed globally for the world to boost \u0026 zap. \n\nWe strive to source local vendors, local artists, local partnerships. Please reach out to us if you are interested in participating in this historic event. ","sig":"f676629d1414d96b464644de6babde0c96bd21ef9b41ba69ad886a1d13a942b855b715b22ccf38bc07fead18d3bdeee82d9e3825cf6f003fb5ff1766d95c70a0"}
	// {"id":"3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5","pubkey":"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d","created_at":1710759386,"kind":1,"tags":[],"content":"Nostr was coopted by our the corporate overlords. It is now featured in https://www.iana.org/assignments/well-known-uris/well-known-uris.xhtml.","sig":"faaec167cca4de50b562b7702e8854e2023f0ccd5f36d1b95b6eac20d352206342d6987e9516d283068c768e94dbe8858e2990c3e05405e707fb6fb771ef92f9"}
}

func ExampleKeyPublic() {
	app.Run([]string{"nak", "key", "public", "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5", "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"})
	// Output:
	// 70f7120d065870513a6bddb61c8d400ad1e43449b1900ffdb5551e4c421375c8
	// 718d756f60cf5179ef35b39dc6db3ff58f04c0734f81f6d4410f0b047ddf9029
}

func ExampleKeyDecrypt() {
	app.Run([]string{"nak", "key", "decrypt", "ncryptsec1qggfep0m5ythsegkmwfrhhx2zx5gazyhdygvlngcds4wsgdpzfy6nr0exy0pdk0ydwrqyhndt2trtwcgwwag0ja3aqclzptfxxqvprdyaz3qfrmazpecx2ff6dph5mfdjnh5sw8sgecul32eru6xet34", "banana"})
	// Output:
	// nsec180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsgyumg0
}

func ExampleRelay() {
	app.Run([]string{"nak", "relay", "relay.nos.social", "pyramid.fiatjaf.com"})
	// Output:
	// {
	//   "name": "nos.social strfry relay",
	//   "description": "This is a strfry instance handled by nos.social",
	//   "pubkey": "89ef92b9ebe6dc1e4ea398f6477f227e95429627b0a33dc89b640e137b256be5",
	//   "contact": "https://nos.social",
	//   "supported_nips": [
	//     1,
	//     2,
	//     4,
	//     9,
	//     11,
	//     12,
	//     16,
	//     20,
	//     22,
	//     28,
	//     33,
	//     40
	//   ],
	//   "software": "git+https://github.com/hoytech/strfry.git",
	//   "version": "0.9.4",
	//   "icon": ""
	// }
	// {
	//   "name": "the fiatjaf pyramid",
	//   "description": "a relay just for the coolest of the coolest",
	//   "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
	//   "contact": "",
	//   "supported_nips": [],
	//   "software": "https://github.com/fiatjaf/khatru",
	//   "version": "n/a",
	//   "limitation": {
	//     "auth_required": false,
	//     "payment_required": false,
	//     "restricted_writes": true
	//   },
	//   "icon": "https://clipart-library.com/images_k/pyramid-transparent/pyramid-transparent-19.png"
	// }
}
