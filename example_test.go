package main

func ExampleEventBasic() {
	app.Run([]string{"nak", "event", "--ts", "1699485669"})
	// Output:
	// {"id":"36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c","pubkey":"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798","created_at":1699485669,"kind":1,"tags":[],"content":"hello from the nostr army knife","sig":"68e71a192e8abcf8582a222434ac823ecc50607450ebe8cc4c145eb047794cc382dc3f888ce879d2f404f5ba6085a47601360a0fa2dd4b50d317bd0c6197c2c2"}
}

func ExampleEventComplex() {
	app.Run([]string{"nak", "event", "--ts", "1699485669", "-k", "11", "-c", "skjdbaskd", "--sec", "17", "-t", "t=spam", "-e", "36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c", "-t", "r=https://abc.def;nothing"})
	// Output:
	// {"id":"aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6","pubkey":"2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f","created_at":1699485669,"kind":11,"tags":[["t","spam"],["r","https://abc.def","nothing"],["e","36d88cf5fcc449f2390a424907023eda7a74278120eebab8d02797cd92e7e29c"]],"content":"skjdbaskd","sig":"1165ac7a27d774d351ef19c8e918fb22f4005fcba193976c3d7edba6ef87ead7f14467f376a9e199f8371835368d86a8506f591e382528d00287fb168a7b8f38"}
}

func ExampleReq() {
	app.Run([]string{"nak", "req", "-k", "1", "-l", "18", "-a", "2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f", "-e", "aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"})
	// Output
	// ["REQ","nak",{"kinds":[1],"authors":["2fa2104d6b38d11b0230010559879124e42ab8dfeff5ff29dc9cdadd4ecacc3f"],"limit":18,"#e":["aec4de6d051a7c2b6ca2d087903d42051a31e07fb742f1240970084822de10a6"]}]
}
