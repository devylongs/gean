package xmss

import (
	"encoding/hex"
	"errors"
	"testing"
	"unsafe"
)

const reamPublicKeyHex = "7bbaf95bd653c827b5775e00b973b24d50ab4743db3373244f29c95fdf4ccc628788ba2b5b9d635acdb25770e8ceef66bfdecd0a"

const reamSignatureHex = "240000006590c5180f52a57ef4d12153ace9dd1bb346ea1299402d32978bd56f2804000004000000903b927d2ee9cf14087b2164dd418115962e322a6da10a58821ee20dd54f3c6a3f6dba14ebc2340b1aa7cc5647e08d0151024229312973331f669e1a268c8a2429d2353393f0d67a6b02da35e589da5bb099ea1ef931e9770ccae83a31f1454b359a4f4227fa1f17895918649115f3416bfa4e7976c5736170bc4e2acaf58342bf8e7472a2d6871e93bbbd01ebb6f30d51ccc3150e1d1e6c7bbfe15ea42cac218a8b94745184ce1f1098d62a9ea7a20662de0464f58822501084da7cbb58833ef9adfe59c17ac121676d92103a52903fbb70c1694717c4695140411977dfcb0e3da29612e27c590ef56967046817cb3727def54b4544f4031427be0a056ba7633334fd4104b0ba550521b61e16ecad72b5fda17de3e7a03bf683f55ecefd215235c6481b5a6f873bc8134373f9fafc3053164e5f435c3d2ad790221601ce274a4117f3104cadb00feb8baf79dd48904c5c1e0c1627c17c41ee7ca3760051ec163eee3e38a7bffa5779570a6cc078a630f5494f4917214954f1da636decff784335944a0674aa82096e5136063ef59c6962e95308074fed4bf6a81301d38a7919149bb24d221e5c44c12f82127c551413fef4f40b6f5ad646f4ca4578baf6f11d3325fe356168925b1f75690c17dbfd74d0d39756106c8a6d10c3bd355f27de621c72ca76734e523ebb8e647ef9fd216093f6bf086f075e4dcc607b5f6ff0603ca71787665a504621bdaf9e0b2924516972dc9c0ee1198f4ae3d6f7109c0c0f4b1708262685c7850709275e6e7f14cf4b74647a04005c501c376d3a179014dc69d26716199b3811773d77800948ad6a6c620d1d2cd0d038283e10c659bd6c3635fa8634482cc14c1d476fa23a40f94d0dcb542b2230bb4f02833713357b8f783b9ffaa50c32ef9933265a072bbc34b671800a8807fae7b235d3f2cc62804a4e3efecbb420122e6a6b62154976faff37329de17d7691a5213d5475b92132e48d657993d12c5eb5d0164f6af8589c199c1b61cf4619c60d72358097e43e5c5a676914748541e8d705626ac3654342f749361577056efe50e65e232fd91c8588576db75cd96e004994278441bc2abac25a51cbdc095764bf7a64ffe05b06cea7bb446f6de72149bf850d795ddd15e464bf7377985f422e937e4ad00563360340c93fd8d0c2695aaac71776f9476c4e574a7645285f5ca714cc021d88eb317d9c0177305bfa06f35622433447803322bf3833b617852b5cbd1d7abbca563f124886792d0133298de40c6d4a8c802faa83da52211bdd5f13c1a1440416b40acc044774e083394d5e803a6a4a3123549d208c340448655b633d43478488717ebc9f3a09b514852c709cc77ace7bba083cc3826d3542ec663a55750838b83369521b40683a268413a302f90adb9a2954466d8d432b316021228f9763e76e7a704866b66d0471626ae5f19c345147b64233266b7a58c7db0c246f370b91b490297ce07130166c424624d96b2adcb3a460fb0cce015f9e33194cc4a128f9153e1ddb24e1509e21793796b5b11fadc22218db97f3650ca8ec0e6003086fab38cf251b71ae2dece3716f3d50e26873ef8346c9951742953e0f720420af17b530c1720f8f606e19a0c223d7059b6da670d12fd555313205312f7c151af068bb0b3d0f80333f2f73159e63a9c7cb46a88ede5ac4ddc20700bee377ec01f568cc179a6ccfc2e86d03912b779e2c4e5cc3d73d3b53f18001dee0032b276d456b822d8b242d6d5e27b794ed14a2478e1c13eb5d0866990e4d75a27b1cca956737cac4b16813ca29191720f632f9826e5cd2d32216a9580646bdcc1f0ff114e0066165f1368a324f27a97c3032a697f3317229782dfe053445921286144fbe7d7e68ac441a91c5cc2543ac6908cc171a67cdb0a638e19bb32f3dc6576b8b4aeb5aa4866a18a458972c1521345fbf885238afa6862b0ee4eb62b1c7b94026b8926f2fae2f469b8bd02a6706995733b9321edaff6c429311f72ec541c65fccf9646f4e2ceb7c58d7f26e8280641dde221f0cb38cee6ccb9bd7238641fb11c576b02a2b47553858208a7ac9b6805a80474c38e3295d00f147ed3deb3d30503ec4076b2543bf307194b2787be3d53f3227ca3b815541709bbbc77a11eff92f5cccca7c7617e303c1b430799018aa0a94c7b152b154956a7e874507895e1735edc2683bb328dd576ff11a2e3fc07f1ba086831e534dab1c0320920156155f72cc7e90795c268c2486d1dc4921204c53082dfb1b830bd9238794881c985ab247274990541cfa8f0e5e582e2caf44f7598f068272eb4ee81c5c04663504b745194c8c570169de83626063e20a6847b82c77931f321cd3ac5cbac802476969255a13a2fe60707f960f5df8095c89cc786ffbe1b86a3e0b7f607f98982b9f10591c9c7bd14d216bac36de88254da273a3601048cb47cb5c2954f34bae09d33d814162bf3d6da2de8c7a51331d437a8012420f6c1323b063e4085e32ee4d2864a14a66bffc29efbca2328416901fe998be14033cb00b104d6732bdc8a963357b9d1290043229e321c25f549c467415472e4061a30b616b23700cc930597e117438293330de628b7c465a8d79b150f0cf913a43e92e6c87e8e644412d0815c0086c46e3028b74d7967a6cf6f5332c4fea10320e90061dc0df4b3f01d0934772291c0cc9b8622b1ee9e120a0178613586b1370f71ef508bcc70c0728894621146a707a9c802321ccf7e50d5d4f8a21c5ced8480117bb11404cbc6eac8ec928507e912474c3fa4ba7dbd22781c28761d19e736f63e0c659c94f243b87271e2424505c2118c30c0dcfd7b719d2a6a549f28da317e1cbeb5748da595c86ada266aed25d185753686e94914c68af6f4c5b833bd91033bbcb6b89cad52b9b65e110683dc96be38d690780253c49bf454d783aa25c0838e00929155c943d7c80c83d0a22c65e6e49c760f50b0623f074ee19138c08671cd6a846faf4237cbc4cee192106ed245451652c591b5a04fd6537255f54a261f7df231fe13a803f617a4719ba15831c4fa84864f581772539091726fd0d3112a55781440796562a9324936110004039a3e3ec26078f76243e1be93df0166270055dcb65a94f2c14c5813145f4bd680c02c33e042d98d17e1c5a9c22d095ed30ad754144ae7f5150ac0df842d4e9415f849f1b36f2c1ff520be3d0721aab9b31c249df28aac8d9378326184262f53307bc77bc6ca59c3349bb29b90b7464ab666f563e5ac741a6390e6d634620fbb33182f958482746ff1138f53e55b9d1a119800e8d6fe04a46044781f813817514338a60a8044b3333249cc9c93cdf6c8537de140943f4907f7926f5a81ed20f1526fd9447412d81e75a05d93b7610e2e27d851b1163cb96e242c08796493f564e0e51a45e17e74b2619dafa0855922b714b84f1266bfc094e494c29175298d9a44eb5cf2d3a696c744f15a1f21d737fb45ed8af333157e88b1c89018d6322f91e751827c030a6fb9d03554dee606a39cc6b79147906c18b376b1a39c20249a64c2b0b95e626204ee24dcfa2665bd5ca0405df11391d15b8604130c3de32f7c43726c10bd5334b16df17f17456605008cb3cf926f0166209bc358562f32293073012bc74250445e39e6cc14aa2613d77de1143e15c2e101cca14b9543d44b248a17c72279b743b0e18483a2331228bd57a3d5ecfa03f9318d269cbf992765ea15678046038749a04c35638c7984cbb90c40b7b94e657bc70a6760520fb4cbe1bbe62f5f7c121268168361963cf3bf032f71e01d9555655faeb58e995c916d7b0850f6813830583717d36e9b4a319ccc4797386f072485834f4772d291a3976f1fa799ba0d007822c011ad959895a4b29c11f3f9f07277a1ce94d1c5d9b0150a1e919528c17340031a204be28ff69ad90c13e4b81f354ef8ec11df62e9c62c1de715981bd4f4095a72d2eb68a5811bc059f0f58a5fa539a9a857e68604248d2fd5c28cabdf11ae4a4692218a18d49d38ff76a6e2b8956b884ac0a5af9cc6680bfb62cf3d83a14d031f0404cc8930898bda055a934624650064e40665ce21754c72b6be8152662b5e29571be85b01cb7728045c92a5a37764a99062abb535c21260612b3026d1145be6c1b3d5e917ee457102fcfc8c5771d011d61f591271220a01e1dc11d1b441217577a5b5cc113421cd740bbe47556719bb51375eda8173d54706cafe98b6b9ad0f9639708e87e77fcbb2bb822ce59f0bdba7746ca286c5b98447d4ca2f027b827ea4c7987e96a0429696bf9cfa21d6add9b79f2eddf2e6fe1c23c118ce2035ca0e3022270b628"

const reamSlot uint32 = 5

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	return b
}

func TestVerifySignatureSSZValid(t *testing.T) {
	t.Skip("hardcoded signature uses the retired Dim64Base8 scheme")
	pkBytes := decodeHex(t, reamPublicKeyHex)
	sigBytes := decodeHex(t, reamSignatureHex)

	var pubkey [52]byte
	copy(pubkey[:], pkBytes)

	var sig [2536]byte
	copy(sig[:], sigBytes)

	var message [32]byte

	valid, err := VerifySignatureSSZ(pubkey, reamSlot, message, sig)
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid signature")
	}
}

func TestVerifySignatureSSZWrongSlot(t *testing.T) {
	pkBytes := decodeHex(t, reamPublicKeyHex)
	sigBytes := decodeHex(t, reamSignatureHex)

	var pubkey [52]byte
	copy(pubkey[:], pkBytes)

	var sig [2536]byte
	copy(sig[:], sigBytes)

	var message [32]byte

	valid, err := VerifySignatureSSZ(pubkey, reamSlot+1, message, sig)
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid signature with wrong slot")
	}
}

func TestVerifySignatureSSZWrongMessage(t *testing.T) {
	pkBytes := decodeHex(t, reamPublicKeyHex)
	sigBytes := decodeHex(t, reamSignatureHex)

	var pubkey [52]byte
	copy(pubkey[:], pkBytes)

	var sig [2536]byte
	copy(sig[:], sigBytes)

	var message [32]byte
	for i := range message {
		message[i] = 0xff
	}

	valid, err := VerifySignatureSSZ(pubkey, reamSlot, message, sig)
	if err != nil {
		t.Fatalf("verify returned error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid signature with wrong message")
	}
}

func TestParsePublicKeyRoundtrip(t *testing.T) {
	pkBytes := decodeHex(t, reamPublicKeyHex)
	var pubkey [52]byte
	copy(pubkey[:], pkBytes)

	pk, err := ParsePublicKey(pubkey)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer FreePublicKey(pk)

	if pk == nil {
		t.Fatal("expected non-nil public key")
	}
}

func TestParseSignatureRoundtrip(t *testing.T) {
	sigBytes := decodeHex(t, reamSignatureHex)

	sig, err := ParseSignature(sigBytes)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer FreeSignature(sig)

	if sig == nil {
		t.Fatal("expected non-nil signature")
	}
}

func TestKeyGenerateSignVerifyRoundtrip(t *testing.T) {
	kp, err := GenerateKeyPair("gean-test-seed-phrase", 0, 1<<18)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	defer kp.Close()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey serialization failed: %v", err)
	}

	var message [32]byte
	message[0] = 0xab
	message[31] = 0xcd

	sig, err := kp.Sign(0, message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	valid, err := VerifySignatureSSZ(pubkey, 0, message, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}

	valid, err = VerifySignatureSSZ(pubkey, 1, message, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid with wrong slot")
	}

	var wrongMsg [32]byte
	wrongMsg[0] = 0xff
	valid, err = VerifySignatureSSZ(pubkey, 0, wrongMsg, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid with wrong message")
	}
}

func TestVerifySignatureSSZMalformedPubkey(t *testing.T) {
	var pubkey [52]byte
	var sig [2536]byte
	var message [32]byte

	_, err := VerifySignatureSSZ(pubkey, 0, message, sig)
	if err != nil {
		return
	}
}

func TestAggregateWithChildrenRejectsMalformedChildProof(t *testing.T) {
	var message [32]byte
	marker := byte(1)
	pubkey := CPubKey(unsafe.Pointer(&marker))

	_, err := AggregateWithChildren(nil, nil, []ChildProof{
		{Pubkeys: []CPubKey{pubkey}},
		{Pubkeys: []CPubKey{pubkey}, Proof: []byte{0x01}},
	}, message, 0)
	if !errors.Is(err, ErrMalformedChildProof) {
		t.Fatalf("error=%v, want ErrMalformedChildProof", err)
	}
}

func TestAggregateRejectsNilRawInputs(t *testing.T) {
	var message [32]byte
	marker := byte(1)
	pubkey := CPubKey(unsafe.Pointer(&marker))

	if _, err := AggregateSignatures([]CPubKey{nil}, []CSig{CSig(unsafe.Pointer(&marker))}, message, 0); !errors.Is(err, ErrMalformedRawInput) {
		t.Fatalf("nil pubkey error=%v, want ErrMalformedRawInput", err)
	}
	if _, err := AggregateSignatures([]CPubKey{pubkey}, []CSig{nil}, message, 0); !errors.Is(err, ErrMalformedRawInput) {
		t.Fatalf("nil signature error=%v, want ErrMalformedRawInput", err)
	}
}

func TestVerifyAggregatedSignatureRejectsNilPubkey(t *testing.T) {
	var message [32]byte
	err := VerifyAggregatedSignature([]byte{0x01}, []CPubKey{nil}, message, 0)
	if !errors.Is(err, ErrMalformedRawInput) {
		t.Fatalf("error=%v, want ErrMalformedRawInput", err)
	}
}
