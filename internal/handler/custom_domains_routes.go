package handler

import "strings"

func (h *CustomDomainHandler) authProxyRoute(host, upstream string) map[string]any {
	return map[string]any{
		"match": []any{map[string]any{"host": []string{host}}},
		"handle": []any{
			map[string]any{
				"handler": "subroute",
				"routes": []any{
					map[string]any{
						"match": []any{map[string]any{"header": map[string][]string{"X-Internal-Secret": {"{env.HIVELOOP_INTERNAL_SECRET}"}}}},
						"handle": []any{map[string]any{
							"handler":   "reverse_proxy",
							"upstreams": []any{map[string]string{"dial": upstream}},
						}},
					},
					map[string]any{
						"handle": []any{map[string]any{"handler": "static_response", "status_code": "403"}},
					},
				},
			},
		},
		"terminal": true,
	}
}

func (h *CustomDomainHandler) simpleProxyRoute(host, upstream string, websocket bool) map[string]any {
	headers := map[string]any{
		"request": map[string]any{
			"set": map[string][]string{
				"X-Real-Ip":         {"{http.request.remote.host}"},
				"X-Forwarded-Proto": {"{http.request.scheme}"},
			},
		},
	}
	if websocket {
		headers["request"].(map[string]any)["set"].(map[string][]string)["Connection"] = []string{"{http.request.header.Connection}"}
		headers["request"].(map[string]any)["set"].(map[string][]string)["Upgrade"] = []string{"{http.request.header.Upgrade}"}
	}
	return map[string]any{
		"match": []any{map[string]any{"host": []string{host}}},
		"handle": []any{map[string]any{
			"handler":   "reverse_proxy",
			"upstreams": []any{map[string]string{"dial": upstream}},
			"headers":   headers,
		}},
		"terminal": true,
	}
}

func (h *CustomDomainHandler) dexRoute() map[string]any {
	return map[string]any{
		"match": []any{map[string]any{"host": []string{"dex.daytona.hiveloop.com"}}},
		"handle": []any{
			map[string]any{
				"handler": "subroute",
				"routes": []any{
					map[string]any{
						"handle": []any{map[string]any{
							"handler": "headers",
							"response": map[string]any{
								"set": map[string][]string{
									"Access-Control-Allow-Origin":  {"https://api.daytona.hiveloop.com"},
									"Access-Control-Allow-Methods": {"GET, OPTIONS"},
									"Access-Control-Allow-Headers": {"Content-Type, Authorization"},
								},
							},
						}},
					},
					map[string]any{
						"match":  []any{map[string]any{"method": []string{"OPTIONS"}}},
						"handle": []any{map[string]any{"handler": "static_response", "status_code": "204"}},
					},
					map[string]any{
						"handle": []any{map[string]any{
							"handler":   "reverse_proxy",
							"upstreams": []any{map[string]string{"dial": "dex:5556"}},
							"headers": map[string]any{
								"request": map[string]any{
									"set": map[string][]string{
										"X-Real-Ip":         {"{http.request.remote.host}"},
										"X-Forwarded-Proto": {"{http.request.scheme}"},
									},
								},
							},
						}},
					},
				},
			},
		},
		"terminal": true,
	}
}

func (h *CustomDomainHandler) previewProxyRoute(host string) map[string]any {
	return map[string]any{
		"match": []any{map[string]any{"host": []string{host}}},
		"handle": []any{map[string]any{
			"handler":   "reverse_proxy",
			"upstreams": []any{map[string]string{"dial": "proxy:4000"}},
			"headers": map[string]any{
				"request": map[string]any{
					"set": map[string][]string{
						"X-Forwarded-Host":               {"{http.request.host}"},
						"X-Daytona-Skip-Preview-Warning": {"true"},
						"X-Real-Ip":                      {"{http.request.remote.host}"},
						"Connection":                     {"{http.request.header.Connection}"},
						"Upgrade":                        {"{http.request.header.Upgrade}"},
					},
				},
			},
			"transport": map[string]any{
				"protocol":     "http",
				"dial_timeout": 30000000000,
			},
		}},
		"terminal": true,
	}
}

func validateDomain(domain string) error {
	if domain == "" {
		return &validationError{"domain is required"}
	}
	if strings.HasPrefix(domain, "*.") {
		return &validationError{"domain should not include wildcard (omit *.)"}
	}
	if strings.Contains(domain, "://") {
		return &validationError{"domain should not include protocol"}
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return &validationError{"domain must have at least two parts (e.g. preview.example.com)"}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string { return e.msg }
