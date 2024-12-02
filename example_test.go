package main

import (
	"context"
)

// these tests are tricky because commands and flags are declared as globals and values set in one call may persist
// to the next. for example, if in the first test we set --limit 2 then doesn't specify --limit in the second then
// it will still return true for cmd.IsSet("limit") and then we will set .LimitZero = true

var ctx = context.Background()

func ExampleEventBasic() {
	app.Run(ctx, []string{"nak", "event", "--ts", "1699485669"})
	// Output:
	// {"kind":1,"id":"36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"tags":[],"content":"hello from the nostr army knife","sig":"68e71a192e8abcf8582a222434ac823ecc50607450ebe8cc4c145eb047794cc382dc3f888ce879d2f404f5ba6085a47601360a0fa2dd4b50d317bd0c6197c2c2"}
}

// (for some reason there can only be one test dealing with stdin in the suite otherwise it halts)
// func ExampleEventParsingFromStdin() {
// 	prevStdin := os.Stdin
// 	defer func() { os.Stdin = prevStdin }()
// 	r, w, _ := os.Pipe()
// 	os.Stdin = r
// 	w.WriteString("{\"content\":\"hello world\"}\n{\"content\":\"hello sun\"}\n")
// 	app.Run(ctx, []string{"nak", "event", "-t", "t=spam", "--ts", "1699485669"})
// 	// Output:
// 	// {"id":"bda134f9077c11973afe6aa5a1cc6f5bcea01c40d318b8f91dcb8e50507cfa52","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[["t","spam"]],"content":"hello world","sig":"7552454bb8e7944230142634e3e34ac7468bad9b21ed6909da572c611018dff1d14d0792e98b5806f6330edc51e09efa6d0b66a9694dc34606c70f4e580e7493"}
// 	// {"id":"879c36ec73acca288825b53585389581d3836e7f0fe4d46e5eba237ca56d6af5","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[["t","spam"]],"content":"hello sun","sig":"6c7e6b13ebdf931d26acfdd00bec2ec1140ddaf8d1ed61453543a14e729a460fe36c40c488ccb194a0e1ab9511cb6c36741485f501bdb93c39ca4c51bc59cbd4"}
// }

func ExampleEventComplex() {
	app.Run(ctx, []string{"nak", "event", "--ts", "1699485669", "-k", "11", "-c", "skjdbaskd", "--sec", "17", "-t", "t=spam", "-e", "36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c", "-t", "r=https://abc.def?name=foobar;nothing"})
	// Output:
	// {"kind":11,"id":"19aba166dcf354bf5ef64f4afe69ada1eb851495001ee05e07d393ee8c8ea179","pubkey":"2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f","created_at":1699485669,"tags":[["t","spam"],["r","https://abc.def?name=foobar","nothing"],["e","36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c"]],"content":"skjdbaskd","sig":"cf452def4a68341c897c3fc96fa34dc6895a5b8cc266d4c041bcdf758ec992ec5adb8b0179e98552aaaf9450526a26d7e62e413b15b1c57e0cfc8db6b29215d7"}
}

func ExampleEncode() {
	app.Run(ctx, []string{"nak", "encode", "npub", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822"})
	app.Run(ctx, []string{"nak", "encode", "nprofile", "-r", "wss://example.com", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822"})
	app.Run(ctx, []string{"nak", "encode", "nprofile", "-r", "wss://example.com", "a6a67ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179822", "a5592173975ded9f836a9572ea8b11a7e16ceb66464d66d50b27163f7f039d2c"})
	// npub156n8a7wuhwk9tgrzjh8gwzc8q2dlekedec5djk0js9d3d7qhnq3qjpdq28
	// nprofile1qqs2dfn7l8wthtz45p3ftn58pvrs9xlumvkuu2xet8egzkcklqtesgspz9mhxue69uhk27rpd4cxcefwvdhk6fl5jug
	// nprofile1qqs2dfn7l8wthtz45p3ftn58pvrs9xlumvkuu2xet8egzkcklqtesgspz9mhxue69uhk27rpd4cxcefwvdhk6fl5jug
	// nprofile1qqs22kfpwwt4mmvlsd4f2uh23vg60ctvadnyvntx659jw93l0upe6tqpz9mhxue69uhk27rpd4cxcefwvdhk64h265a
}

func ExampleDecode() {
	app.Run(ctx, []string{"nak", "decode", "naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu", "nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8"})
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

func ExampleDecodePubkey() {
	app.Run(ctx, []string{"nak", "decode", "-p", "npub10xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqpkge6d", "npub1ccz8l9zpa47k6vz9gphftsrumpw80rjt3nhnefat4symjhrsnmjs38mnyd"})
	// Output:
	// 79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798
	// c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5
}

func ExampleDecodeEventId() {
	app.Run(ctx, []string{"nak", "decode", "-e", "nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8", "nevent1qqswh48lurxs8u0pll9qj2rzctvjncwhstpzlstq59rdtzlty79awns5hl5uf"})
	// Output:
	// 3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5
	// ebd4ffe0cd03f1e1ffca092862c2d929e1d782c22fc160a146d58beb278bd74e
}

func ExampleReq() {
	app.Run(ctx, []string{"nak", "req", "-k", "1", "-l", "18", "-a", "2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f", "-e", "aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"})
	// Output:
	// ["REQ","nak",{"kinds":[1],"authors":["2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f"],"limit":18,"#e":["aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"]}]
}

func ExampleMultipleFetch() {
	app.Run(ctx, []string{"nak", "fetch", "naddr1qqyrgcmyxe3kvefhqyxhwumn8ghj7mn0wvhxcmmvqgs9kqvr4dkruv3t7n2pc6e6a7v9v2s5fprmwjv4gde8c4fe5y29v0srqsqqql9ngrt6tu", "nevent1qyd8wumn8ghj7urewfsk66ty9enxjct5dfskvtnrdakj7qgmwaehxw309aex2mrp0yh8wetnw3jhymnzw33jucm0d5hszxthwden5te0wfjkccte9eekummjwsh8xmmrd9skctcpzamhxue69uhkzarvv9ejumn0wd68ytnvv9hxgtcqyqllp5v5j0nxr74fptqxkhvfv0h3uj870qpk3ln8a58agyxl3fka296ewr8"})
	// Output:
	// {"kind":31923,"id":"9ae5014573fc75ced00b343868d2cd9343ebcbbae50591c6fa8ae1cd99568f05","pubkey":"5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","created_at":1707764605,"tags":[["d","4cd6cfe7"],["name","Nostr PHX Presents Culture Shock"],["description","Nostr PHX presents Culture Shock the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, food, drinks, conversation, vendors and best of all, a live convert which will stream globally for the world to zap. "],["start","1708185600"],["end","1708228800"],["start_tzid","America/Phoenix"],["p","5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e","","host"],["location","Hello Merch, 850 W Lincoln St, Phoenix, AZ 85007, USA","Hello Merch","850 W Lincoln St, Phoenix, AZ 85007, USA"],["address","Hello Merch, 850 W Lincoln St, Phoenix, AZ 85007, USA","Hello Merch","850 W Lincoln St, Phoenix, AZ 85007, USA"],["g","9tbq1rzn"],["image","https://flockstr.s3.amazonaws.com/event/15vSaiscDhVH1KBXhA0i8"],["about","Nostr PHX presents Culture Shock : the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, conversation, food and goods which will be capped off with a live concert streamed globally for the world to boost \u0026 zap. \n\nWe strive to source local vendors, local artists, local partnerships. Please reach out to us if you are interested in participating in this historic event. "],["calendar","31924:5b0183ab6c3e322bf4d41c6b3aef98562a144847b7499543727c5539a114563e:1f238c94"]],"content":"Nostr PHX presents Culture Shock : the first Value 4 Value Cultural Event in Downtown Phoenix. We will showcase the power of Nostr + Bitcoin / Lightning with a full day of education, conversation, food and goods which will be capped off with a live concert streamed globally for the world to boost \u0026 zap. \n\nWe strive to source local vendors, local artists, local partnerships. Please reach out to us if you are interested in participating in this historic event. ","sig":"f676629d1414d96b464644de6babde0c96bd21ef9b41ba69ad886a1d13a942b855b715b22ccf38bc07fead18d3bdeee82d9e3825cf6f003fb5ff1766d95c70a0"}
	// {"kind":1,"id":"3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5","pubkey":"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d","created_at":1710759386,"tags":[],"content":"Nostr was coopted by our the corporate overlords. It is now featured in https://www.iana.org/assignments/well-known-uris/well-known-uris.xhtml.","sig":"faaec167cca4de50b562b7702e8854e2023f0ccd5f36d1b95b6eac20d352206342d6987e9516d283068c768e94dbe8858e2990c3e05405e707fb6fb771ef92f9"}
}

func ExampleKeyPublic() {
	app.Run(ctx, []string{"nak", "key", "public", "3ff0d19493e661faa90ac06b5d8963ef1e48fe780368fe67ed0fd410df8a6dd5", "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"})
	// Output:
	// 70f7120d065870513a6bddb61c8d400ad1e43449b1900ffdb5551e4c421375c8
	// 718d756f60cf5179ef35b39dc6db3ff58f04c0734f81f6d4410f0b047ddf9029
}

func ExampleKeyDecrypt() {
	app.Run(ctx, []string{"nak", "key", "decrypt", "ncryptsec1qgg2gx2a7hxpsse2zulrv7m8qwccvl3mh8e9k8vtz3wpyrwuuclaq73gz7ddt5kpa93qyfhfjakguuf8uhw90jn6mszh7kqeh9mxzlyw8hy75fluzx4h75frwmu2yngsq7hx7w32d0vdyxyns5g6rqft", "banana"})
	// Output:
	// 718d756f60cf5179ef35b39dc6db3ff58f04c0734f81f6d4410f0b047ddf9029
}

func ExampleReqIdFromRelay() {
	app.Run(ctx, []string{"nak", "req", "-i", "20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da", "--limit", "1", "nos.lol"})
	// Output:
	// {"kind":1,"id":"20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da","pubkey":"dd664d5e4016433a8cd69f005ae1480804351789b59de5af06276de65633d319","created_at":1720972243,"tags":[["e","bdb2210fe6d9c4b141f08b5d9d1147cd8e1dc1d82f552a889ab171894249d21d","","root"],["e","c2e45f09e7d62ed12afe2b8b1bcf6be823b560a53ef06905365a78979a1b9ee3","","reply"],["p","036533caa872376946d4e4fdea4c1a0441eda38ca2d9d9417bb36006cbaabf58","","mention"]],"content":"Yeah, so bizarre, but I guess most people are meant to be serfs.","sig":"9ea7488415c250d0ac8fcb2219f211cb369dddf2a75c0f63d2db773c6dc1ef9dd9679b8941c0e7551744ea386afebad2024be8ce3ac418d4f47c95e7491af38e"}
}

func ExampleReqWithFlagsAfter1() {
	app.Run(ctx, []string{"nak", "req", "nos.lol", "-i", "20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da", "--limit", "1"})
	// Output:
	// {"kind":1,"id":"20a6606ed548fe7107533cf3416ce1aa5e957c315c2a40249e12bd9873dca7da","pubkey":"dd664d5e4016433a8cd69f005ae1480804351789b59de5af06276de65633d319","created_at":1720972243,"tags":[["e","bdb2210fe6d9c4b141f08b5d9d1147cd8e1dc1d82f552a889ab171894249d21d","","root"],["e","c2e45f09e7d62ed12afe2b8b1bcf6be823b560a53ef06905365a78979a1b9ee3","","reply"],["p","036533caa872376946d4e4fdea4c1a0441eda38ca2d9d9417bb36006cbaabf58","","mention"]],"content":"Yeah, so bizarre, but I guess most people are meant to be serfs.","sig":"9ea7488415c250d0ac8fcb2219f211cb369dddf2a75c0f63d2db773c6dc1ef9dd9679b8941c0e7551744ea386afebad2024be8ce3ac418d4f47c95e7491af38e"}
}

func ExampleReqWithFlagsAfter2() {
	app.Run(ctx, []string{"nak", "req", "-e", "893d4c10f1c230240812c6bdf9ad877eed1e29e87029d153820c24680bb183b1", "nostr.mom", "--author", "2a7dcf382bcc96a393ada5c975f500393b3f7be6e466bff220aa161ad6b15eb6", "--limit", "1", "-k", "7"})
	// Output:
	// {"kind":7,"id":"9b4868b068ea34ae51092807586c4541b3569d9efc23862aea48ef13de275857","pubkey":"2a7dcf382bcc96a393ada5c975f500393b3f7be6e466bff220aa161ad6b15eb6","created_at":1720987327,"tags":[["e","893d4c10f1c230240812c6bdf9ad877eed1e29e87029d153820c24680bb183b1"],["p","1e978baae414eee990dba992871549ad4a099b9d6f7e71c8059b254ea024dddc"],["k","1"]],"content":"❤️","sig":"7eddd112c642ecdb031330dadc021790642b3c10ecc64158ba3ae63edd798b26afb9b5a3bba72835ce171719a724de1472f65c9b3339b6bead0ce2846f93dfc9"}
}

func ExampleReqWithFlagsAfter3() {
	app.Run(ctx, []string{"nak", "req", "--limit", "1", "pyramid.fiatjaf.com", "-a", "3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24", "-qp", "3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24", "-e", "9f3c1121c96edf17d84b9194f74d66d012b28c4e25b3ef190582c76b8546a188"})
	// Output:
	// {"kind":1,"id":"101572c80ebdc963dab8440f6307387a3023b6d90f7e495d6c5ee1ef77045a67","pubkey":"3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24","created_at":1720987305,"tags":[["e","ceacdc29fa7a0b51640b30d2424e188215460617db5ba5bb52d3fbf0094eebb3","","root"],["e","9f3c1121c96edf17d84b9194f74d66d012b28c4e25b3ef190582c76b8546a188","","reply"],["p","3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24"],["p","6b96c3eb36c6cd457d906bbaafe7b36cacfb8bcc4ab235be6eab3b71c6669251"]],"content":"Nope. I grew up playing in the woods. Never once saw a bear in the woods. If I did, I'd probably shiy my pants, then scream at it like I was a crazy person with my arms above my head to make me seem huge.","sig":"b098820b4a5635865cada9f9a5813be2bc6dd7180e16e590cf30e07916d8ed6ed98ab38b64f3bfba12d88d37335f229f7ef8c084bc48132e936c664a54d3e650"}
}

func ExampleNaturalTimestamps() {
	app.Run(ctx, []string{"nak", "event", "-t", "plu=pla", "-e", "3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24", "--ts", "May 19 2018 03:37:19", "-c", "nn"})
	// Output:
	// {"kind":0,"id":"b10da0095f96aa2accd99fa3d93bf29a76f51d2594cf5a0a52f8e961aecd0b67","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1526711839,"tags":[["plu","pla"],["e","3f770d65d3a764a9c5cb503ae123e62ec7598ad035d836e2a810f3877a745b24"]],"content":"nn","sig":"988442c97064a041ba5e2bfbd64e84d3f819b2169e865511d9d53e74667949ff165325942acaa2ca233c8b529adedf12cf44088cf04081b56d098c5f4d52dd8f"}
}
