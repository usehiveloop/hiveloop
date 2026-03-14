package proxy

import "net/http"

// AttachAuth sets the appropriate authentication header or query parameter
// on the outbound request based on the credential's auth scheme.
func AttachAuth(req *http.Request, scheme string, apiKey []byte) {
	switch scheme {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+string(apiKey))
	case "x-api-key":
		req.Header.Set("x-api-key", string(apiKey))
	case "api-key":
		req.Header.Set("api-key", string(apiKey))
	case "query_param":
		q := req.URL.Query()
		q.Set("key", string(apiKey))
		req.URL.RawQuery = q.Encode()
	}
}
