package github

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/nacl/box"
)

//
// Note from Chelsea:
// GitHub requires clients to encrypt secrets using a library called
// libsodium in REST api requests. This process is apparently only a few
// lines of code in Ruby and Python (see docs and code samples here:
// https://docs.github.com/en/rest/guides/encrypting-secrets-for-the-rest-api)
// but of course it's more complicated in Go.
//
// This helpful blog post provides some background on the best approach:
// https://zostay.com/posts/2022/05/04/do-not-use-libsodium-with-go/
//
// It lead to me to a small library, https://github.com/jefflinse/githubsecret,
// which I've imported-by-copy here.
//

//
// =========================================================================
// All code below was copied from https://github.com/jefflinse/githubsecret.
// =========================================================================
//

const (
	keySize   = 32
	nonceSize = 24
)

// Test indirection
var generateKey = box.GenerateKey

// Encrypt encrypts a secret using the provided recipient public key.
func Encrypt(recipientPublicKey string, content string) (string, error) {
	// decode the provided public key from base64
	recipientKey := new([keySize]byte)
	b, err := base64.StdEncoding.DecodeString(recipientPublicKey)
	if err != nil {
		return "", err
	} else if size := len(b); size != keySize {
		return "", fmt.Errorf("recipient public key has invalid length (%d bytes)", size)
	}

	copy(recipientKey[:], b)

	// create an ephemeral key pair
	pubKey, privKey, err := generateKey(rand.Reader)
	if err != nil {
		return "", err
	}

	// create the nonce by hashing together the two public keys
	nonce := new([nonceSize]byte)
	nonceHash, err := blake2b.New(nonceSize, nil)
	if err != nil {
		return "", err
	}

	if _, err := nonceHash.Write(pubKey[:]); err != nil {
		return "", err
	}

	if _, err := nonceHash.Write(recipientKey[:]); err != nil {
		return "", err
	}

	copy(nonce[:], nonceHash.Sum(nil))

	// begin the output with the ephemeral public key and append the encrypted content
	out := box.Seal(pubKey[:], []byte(content), nonce, recipientKey, privKey)

	// base64-encode the final output
	return base64.StdEncoding.EncodeToString(out), nil
}
