package did

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

func GenerateKeyPair() (publicKeyBase64, privateKeyBase64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

func SignMessage(privateKeyBase64, message string) (string, error) {
	privBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return "", err
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key size")
	}

	sig := ed25519.Sign(ed25519.PrivateKey(privBytes), []byte(message))
	return base64.StdEncoding.EncodeToString(sig), nil
}

func VerifyMessage(publicKeyBase64, message, signatureBase64 string) (bool, error) {
	pubBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return false, err
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false, err
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return false, errors.New("invalid public key size")
	}

	ok := ed25519.Verify(ed25519.PublicKey(pubBytes), []byte(message), sigBytes)
	return ok, nil
}
