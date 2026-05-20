package handler

import "testing"

func TestWebhookSignature_MatchesBridgeGoldenVector(t *testing.T) {
	payload := []byte("[]")
	secret := "webhook-secret"
	timestamp := int64(1700000000)
	const bridgeSignature = "UtY1lkos+DpA1Pd8wcLPLzCodQ6LHJNe/Be+DfGPFTM="

	if !verifyWebhookSignature(payload, secret, timestamp, bridgeSignature) {
		t.Fatal("Go bridge webhook verifier rejected the Bridge golden signature")
	}
	if verifyWebhookSignature(payload, "wrong-secret", timestamp, bridgeSignature) {
		t.Fatal("Go bridge webhook verifier accepted the golden signature with the wrong secret")
	}
}
