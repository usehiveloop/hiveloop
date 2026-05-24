package config

import "testing"

func TestRuntimeControlPlaneBaseURLPrefersAPIWebhookBaseURL(t *testing.T) {
	cfg := &Config{
		APIWebhookBaseURL:     "http://host.docker.internal:8080/",
		SpecialistSandboxHost: "api.usehivy.com",
	}
	if got := cfg.RuntimeControlPlaneBaseURL(); got != "http://host.docker.internal:8080" {
		t.Fatalf("RuntimeControlPlaneBaseURL = %q", got)
	}
}

func TestRuntimeControlPlaneBaseURLFallsBackToSandboxHost(t *testing.T) {
	cfg := &Config{SpecialistSandboxHost: "api.hivy.test"}
	if got := cfg.RuntimeControlPlaneBaseURL(); got != "https://api.hivy.test" {
		t.Fatalf("RuntimeControlPlaneBaseURL = %q", got)
	}
}
