package middleware

import "testing"

func TestShouldCaptureProxyGeneration(t *testing.T) {
	if shouldCaptureProxyGeneration(&TokenClaims{TokenType: "employee_proxy"}) {
		t.Fatal("employee runtime proxy calls should be captured from model_usage webhooks")
	}
	if !shouldCaptureProxyGeneration(&TokenClaims{TokenType: "embedding_proxy"}) {
		t.Fatal("non-employee proxy calls should still be captured by proxy middleware")
	}
	if shouldCaptureProxyGeneration(nil) {
		t.Fatal("nil claims should not be captured")
	}
}
