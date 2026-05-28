package config

import "testing"

func TestProxyOpenAIBaseURL_DefaultsToProductionV1(t *testing.T) {
	cfg := &Config{}
	if got := cfg.ProxyOpenAIBaseURL(); got != "https://proxy.usehivy.com/v1" {
		t.Fatalf("ProxyOpenAIBaseURL = %q", got)
	}
}

func TestProxyOpenAIBaseURL_PreservesConfiguredScheme(t *testing.T) {
	cfg := &Config{ProxyHost: "http://host.docker.internal:8080"}
	if got := cfg.ProxyOpenAIBaseURL(); got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("ProxyOpenAIBaseURL = %q", got)
	}
}

func TestProxyOpenAIBaseURL_StripsProxyMount(t *testing.T) {
	cfg := &Config{ProxyHost: "http://host.docker.internal:8080/v1/proxy"}
	if got := cfg.ProxyOpenAIBaseURL(); got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("ProxyOpenAIBaseURL = %q", got)
	}
}

func TestProxyOpenAIBaseURL_StripsProxyMountV1(t *testing.T) {
	cfg := &Config{ProxyHost: "http://host.docker.internal:8080/v1/proxy/v1"}
	if got := cfg.ProxyOpenAIBaseURL(); got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("ProxyOpenAIBaseURL = %q", got)
	}
}

func TestProxyOriginURL_AddsHTTPSForHostOnlyConfig(t *testing.T) {
	cfg := &Config{ProxyHost: "proxy.hivy.test"}
	if got := cfg.ProxyOriginURL(); got != "https://proxy.hivy.test" {
		t.Fatalf("ProxyOriginURL = %q", got)
	}
}
