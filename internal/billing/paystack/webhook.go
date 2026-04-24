package paystack

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
)

// signatureHeader is the webhook signature header Paystack sets. Lookup via
// http.Header is case-insensitive so "X-Paystack-Signature" matches whatever
// casing Paystack uses on the wire.
const signatureHeader = "X-Paystack-Signature"

// errInvalidSignature is the sentinel returned from VerifyWebhook on any
// failure — header missing, wrong algorithm, tampered body.
var errInvalidSignature = errors.New("paystack webhook: invalid signature")

// VerifyWebhook authenticates an incoming Paystack webhook.
//
// Paystack signs the raw request body with HMAC-SHA512 keyed by the merchant
// secret key, hex-encoded, in the X-Paystack-Signature header. We must hash
// the raw bytes before any JSON parse/re-serialise — our caller passes in
// body as originally read.
//
// This method does not read r.Body; the caller already buffered it.
func (p *Provider) VerifyWebhook(r *http.Request, body []byte) error {
	got := r.Header.Get(signatureHeader)
	if got == "" {
		return errInvalidSignature
	}
	mac := hmac.New(sha512.New, []byte(p.cfg.SecretKey))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(want)) {
		return errInvalidSignature
	}
	return nil
}
