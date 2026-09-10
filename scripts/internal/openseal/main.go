package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"golang.org/x/crypto/nacl/box"
)

func main() {
	pubB64 := flag.String("pub", "", "public key base64")
	privB64 := flag.String("priv", "", "private key base64")
	ctB64 := flag.String("ct", "", "ciphertext base64")
	flag.Parse()
	pub := must32(*pubB64)
	priv := must32(*privB64)
	ct, err := base64.StdEncoding.DecodeString(*ctB64)
	if err != nil {
		fail(err)
	}
	msg, ok := box.OpenAnonymous(nil, ct, pub, priv)
	if !ok {
		fail(fmt.Errorf("OpenAnonymous failed"))
	}
	fmt.Println(string(msg))
}

func must32(b64 string) *[32]byte {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != 32 {
		fail(fmt.Errorf("key: %v len=%d", err, len(raw)))
	}
	var out [32]byte
	copy(out[:], raw)
	return &out
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
