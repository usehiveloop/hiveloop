package sentry

import (
	"net/http"

	sentryhttpclient "github.com/getsentry/sentry-go/httpclient"
)

func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	if !Enabled() {
		if rt == nil {
			return http.DefaultTransport
		}
		return rt
	}
	if rt == nil {
		rt = http.DefaultTransport
	}
	return sentryhttpclient.NewSentryRoundTripper(rt)
}

func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		return nil
	}
	if !Enabled() {
		return client
	}
	client.Transport = WrapTransport(client.Transport)
	return client
}
