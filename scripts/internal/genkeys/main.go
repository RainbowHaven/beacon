// Command genkeys creates a Curve25519 sealed-box keypair for local Beacon use.
// Private key material is written only to paths you pass (or stdout via -export);
// never commit those files.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/nacl/box"
)

func main() {
	outDir := flag.String("out-dir", "", "write identity.pub.b64 and identity.priv.b64 here")
	export := flag.Bool("export", false, "print shell exports for IDENTITY_PUBLIC_KEY_B64 and IDENTITY_PRIVATE_KEY_B64")
	flag.Parse()

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		os.Exit(1)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])
	privB64 := base64.StdEncoding.EncodeToString(priv[:])

	if *export {
		fmt.Printf("export IDENTITY_PUBLIC_KEY_B64=%q\n", pubB64)
		fmt.Printf("export IDENTITY_PRIVATE_KEY_B64=%q\n", privB64)
		return
	}
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: genkeys -out-dir .local   OR   genkeys -export")
		os.Exit(2)
	}
	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	pubPath := filepath.Join(*outDir, "identity.pub.b64")
	privPath := filepath.Join(*outDir, "identity.priv.b64")
	if err := os.WriteFile(pubPath, []byte(pubB64+"\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write pub: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(privPath, []byte(privB64+"\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write priv: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", pubPath)
	fmt.Fprintf(os.Stderr, "Wrote %s (keep offline; never commit)\n", privPath)
}
