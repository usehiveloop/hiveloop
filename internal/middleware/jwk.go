package middleware

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
)

// parseRSAPublicKey builds an *rsa.PublicKey from base64url-encoded JWK fields.
func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decoding RSA n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decoding RSA e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// parseECPublicKey builds an *ecdsa.PublicKey from base64url-encoded JWK fields.
func parseECPublicKey(crv, xB64, yB64 string) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve: %s", crv)
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, fmt.Errorf("decoding EC x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, fmt.Errorf("decoding EC y: %w", err)
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}
