package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *CustomDomainHandler) registerAcmeDNS() (*acmeDNSRegisterResponse, error) {
	if h.cfg.AcmeDNSAPIURL == "" {
		return nil, fmt.Errorf("ACME_DNS_API_URL not configured")
	}

	req, err := http.NewRequest("POST", h.cfg.AcmeDNSAPIURL+"/register", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Secret", h.cfg.InternalDomainSecret)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("acme-dns request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("acme-dns returned %d: %s", resp.StatusCode, string(body))
	}

	var result acmeDNSRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode acme-dns response: %w", err)
	}

	return &result, nil
}

func (h *CustomDomainHandler) reloadCaddyConfig() error {
	if h.cfg.CaddyAdminURL == "" {
		return fmt.Errorf("CADDY_ADMIN_URL not configured")
	}

	var domains []model.CustomDomain
	if err := h.db.Where("verified = true").Find(&domains).Error; err != nil {
		return fmt.Errorf("failed to fetch verified domains: %w", err)
	}

	config := h.buildCaddyConfig(domains)

	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal Caddy config: %w", err)
	}

	req, err := http.NewRequest("POST", h.cfg.CaddyAdminURL+"/load", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", h.cfg.InternalDomainSecret)

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("caddy /load request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy /load returned %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("caddy config reloaded", "custom_domains", len(domains))
	return nil
}

func (h *CustomDomainHandler) buildCaddyConfig(customDomains []model.CustomDomain) map[string]any {
	routes := []any{}

	routes = append(routes, h.authProxyRoute("acme-dns-api.daytona.hiveloop.com", "acme-dns:443"))
	routes = append(routes, h.authProxyRoute("caddy-admin.daytona.hiveloop.com", "localhost:2019"))
	routes = append(routes, h.simpleProxyRoute("api.daytona.hiveloop.com", "api:3000", true))
	routes = append(routes, h.dexRoute())
	routes = append(routes, h.previewProxyRoute("*.preview.hiveloop.com"))

	for _, cd := range customDomains {
		routes = append(routes, h.previewProxyRoute("*."+cd.Domain))
	}

	staticSubjects := []string{
		"acme-dns-api.daytona.hiveloop.com",
		"caddy-admin.daytona.hiveloop.com",
		"api.daytona.hiveloop.com",
		"dex.daytona.hiveloop.com",
		"*.preview.hiveloop.com",
	}

	policies := []any{
		map[string]any{
			"subjects": staticSubjects,
			"issuers": []any{
				map[string]any{
					"module": "acme",
					"email":  "admin@hiveloop.com",
					"challenges": map[string]any{
						"dns": map[string]any{
							"provider": map[string]any{
								"name":      "cloudflare",
								"api_token": "{env.CLOUDFLARE_API_TOKEN}",
							},
						},
					},
				},
			},
		},
	}

	if len(customDomains) > 0 {
		customSubjects := make([]string, len(customDomains))
		acmeDNSConfig := map[string]any{}

		for i, cd := range customDomains {
			wildcardDomain := "*." + cd.Domain
			customSubjects[i] = wildcardDomain
			acmeDNSConfig[cd.Domain] = map[string]any{
				"server_url": "http://acme-dns:443",
				"username":   cd.AcmeDNSUsername,
				"password":   cd.AcmeDNSPassword,
				"subdomain":  cd.AcmeDNSSubdomain,
			}
		}

		policies = append(policies, map[string]any{
			"subjects": customSubjects,
			"issuers": []any{
				map[string]any{
					"module": "acme",
					"email":  "admin@hiveloop.com",
					"challenges": map[string]any{
						"dns": map[string]any{
							"provider": map[string]any{
								"name":   "acmedns",
								"config": acmeDNSConfig,
							},
							"resolvers": []string{"1.1.1.1:53", "8.8.8.8:53"},
						},
					},
				},
			},
		})
	}

	automate := make([]string, len(customDomains))
	for i, cd := range customDomains {
		automate[i] = "*." + cd.Domain
	}

	config := map[string]any{
		"admin": map[string]any{
			"listen": "0.0.0.0:2019",
		},
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv0": map[string]any{
						"listen": []string{":443"},
						"routes": routes,
					},
				},
			},
			"tls": map[string]any{
				"automation": map[string]any{
					"policies": policies,
				},
			},
		},
	}

	if len(automate) > 0 {
		config["apps"].(map[string]any)["tls"].(map[string]any)["certificates"] = map[string]any{
			"automate": automate,
		}
	}

	return config
}
