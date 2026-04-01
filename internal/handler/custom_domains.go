package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/config"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// CustomDomainHandler manages custom preview domain configuration.
type CustomDomainHandler struct {
	db     *gorm.DB
	cfg    *config.Config
	client *http.Client
}

// NewCustomDomainHandler creates a new custom domain handler.
func NewCustomDomainHandler(db *gorm.DB, cfg *config.Config) *CustomDomainHandler {
	return &CustomDomainHandler{
		db:     db,
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type createDomainRequest struct {
	Domain string `json:"domain"`
}

type createDomainResponse struct {
	model.CustomDomain
	DNSRecords []dnsRecord `json:"dns_records"`
}

type dnsRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type verifyDomainResponse struct {
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

// acme-dns registration response
type acmeDNSRegisterResponse struct {
	FullDomain string `json:"fulldomain"`
	SubDomain  string `json:"subdomain"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// Create handles POST /v1/custom-domains.
func (h *CustomDomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if err := validateDomain(domain); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var existing model.CustomDomain
	if err := h.db.Where("domain = ?", domain).First(&existing).Error; err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "domain already registered"})
		return
	}

	acmeReg, err := h.registerAcmeDNS()
	if err != nil {
		slog.Error("acme-dns registration failed", "error", err, "domain", domain)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register domain for certificate challenges"})
		return
	}

	cd := model.CustomDomain{
		OrgID:            org.ID,
		Domain:           domain,
		CNAMETarget:      h.cfg.PreviewCNAMETarget,
		AcmeDNSSubdomain: acmeReg.SubDomain,
		AcmeDNSUsername:  acmeReg.Username,
		AcmeDNSPassword:  acmeReg.Password,
		AcmeDNSServerURL: h.cfg.AcmeDNSAPIURL,
	}

	if err := h.db.Create(&cd).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create domain"})
		return
	}

	writeJSON(w, http.StatusCreated, createDomainResponse{
		CustomDomain: cd,
		DNSRecords: []dnsRecord{
			{Type: "CNAME", Name: "*." + domain, Value: h.cfg.PreviewCNAMETarget},
			{Type: "CNAME", Name: "_acme-challenge." + domain, Value: cd.AcmeChallengeCNAME()},
		},
	})
}

// List handles GET /v1/custom-domains.
func (h *CustomDomainHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var domains []model.CustomDomain
	if err := h.db.Where("org_id = ?", org.ID).Order("created_at DESC").Find(&domains).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list domains"})
		return
	}

	type domainWithRecords struct {
		model.CustomDomain
		DNSRecords []dnsRecord `json:"dns_records"`
	}

	result := make([]domainWithRecords, len(domains))
	for i, d := range domains {
		result[i] = domainWithRecords{
			CustomDomain: d,
			DNSRecords: []dnsRecord{
				{Type: "CNAME", Name: "*." + d.Domain, Value: d.CNAMETarget},
				{Type: "CNAME", Name: "_acme-challenge." + d.Domain, Value: d.AcmeChallengeCNAME()},
			},
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// Verify handles POST /v1/custom-domains/{id}/verify.
func (h *CustomDomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain ID"})
		return
	}

	var cd model.CustomDomain
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&cd).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	// Check traffic CNAME
	trafficHost := "verify-check." + cd.Domain
	trafficCNAME, err := net.LookupCNAME(trafficHost)
	if err != nil {
		writeJSON(w, http.StatusOK, verifyDomainResponse{
			Verified: false,
			Message:  fmt.Sprintf("DNS lookup failed for %s. Create CNAME: *.%s → %s", trafficHost, cd.Domain, cd.CNAMETarget),
		})
		return
	}
	trafficCNAME = strings.TrimSuffix(trafficCNAME, ".")
	if !strings.EqualFold(trafficCNAME, cd.CNAMETarget) {
		writeJSON(w, http.StatusOK, verifyDomainResponse{
			Verified: false,
			Message:  fmt.Sprintf("Traffic CNAME points to %s, expected %s", trafficCNAME, cd.CNAMETarget),
		})
		return
	}

	// Check challenge CNAME
	challengeHost := "_acme-challenge." + cd.Domain
	challengeCNAME, err := net.LookupCNAME(challengeHost)
	if err != nil {
		writeJSON(w, http.StatusOK, verifyDomainResponse{
			Verified: false,
			Message:  fmt.Sprintf("DNS lookup failed for %s. Create CNAME: %s → %s", challengeHost, challengeHost, cd.AcmeChallengeCNAME()),
		})
		return
	}
	challengeCNAME = strings.TrimSuffix(challengeCNAME, ".")
	expectedChallenge := cd.AcmeChallengeCNAME()
	if !strings.EqualFold(challengeCNAME, expectedChallenge) {
		writeJSON(w, http.StatusOK, verifyDomainResponse{
			Verified: false,
			Message:  fmt.Sprintf("Challenge CNAME points to %s, expected %s", challengeCNAME, expectedChallenge),
		})
		return
	}

	// Mark verified
	now := time.Now()
	cd.Verified = true
	cd.VerifiedAt = &now
	h.db.Save(&cd)

	// Reload Caddy with all verified domains
	if err := h.reloadCaddyConfig(); err != nil {
		slog.Error("failed to reload Caddy config", "error", err, "domain", cd.Domain)
		// Domain is verified in DB even if Caddy reload fails — next verify/delete will retry
		writeJSON(w, http.StatusOK, verifyDomainResponse{
			Verified: true,
			Message:  "Domain verified. TLS provisioning will retry shortly.",
		})
		return
	}

	writeJSON(w, http.StatusOK, verifyDomainResponse{
		Verified: true,
		Message:  "Domain verified and wildcard TLS certificate is being provisioned",
	})
}

// Delete handles DELETE /v1/custom-domains/{id}.
func (h *CustomDomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid domain ID"})
		return
	}

	var cd model.CustomDomain
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&cd).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	wasVerified := cd.Verified
	h.db.Delete(&cd)

	if wasVerified {
		if err := h.reloadCaddyConfig(); err != nil {
			slog.Error("failed to reload Caddy config after domain deletion", "error", err, "domain", cd.Domain)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// registerAcmeDNS registers a new subdomain with the acme-dns server.
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

// reloadCaddyConfig builds the full Caddy JSON config from the database
// and POSTs it to the Caddy admin API /load endpoint for atomic replacement.
func (h *CustomDomainHandler) reloadCaddyConfig() error {
	if h.cfg.CaddyAdminURL == "" {
		return fmt.Errorf("CADDY_ADMIN_URL not configured")
	}

	// Fetch all verified custom domains
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

// buildCaddyConfig generates the complete Caddy JSON config.
func (h *CustomDomainHandler) buildCaddyConfig(customDomains []model.CustomDomain) map[string]any {
	// --- Routes ---
	routes := []any{}

	// Static: acme-dns API proxy
	routes = append(routes, h.authProxyRoute("acme-dns-api.daytona.llmvault.dev", "acme-dns:443"))

	// Static: Caddy admin API proxy
	routes = append(routes, h.authProxyRoute("caddy-admin.daytona.llmvault.dev", "localhost:2019"))

	// Static: API + Dashboard
	routes = append(routes, h.simpleProxyRoute("api.daytona.llmvault.dev", "api:3000", true))

	// Static: Dex OIDC (with CORS)
	routes = append(routes, h.dexRoute())

	// Static: Primary preview domain
	routes = append(routes, h.previewProxyRoute("*.preview.llmvault.dev"))

	// Dynamic: Custom preview domains
	for _, cd := range customDomains {
		routes = append(routes, h.previewProxyRoute("*."+cd.Domain))
	}

	// --- TLS Automation Policies ---
	// Policy 0: Static domains via Cloudflare DNS
	staticSubjects := []string{
		"acme-dns-api.daytona.llmvault.dev",
		"caddy-admin.daytona.llmvault.dev",
		"api.daytona.llmvault.dev",
		"dex.daytona.llmvault.dev",
		"*.preview.llmvault.dev",
	}

	policies := []any{
		map[string]any{
			"subjects": staticSubjects,
			"issuers": []any{
				map[string]any{
					"module": "acme",
					"email":  "admin@llmvault.dev",
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

	// Policy 1: Custom domains via acme-dns (if any)
	if len(customDomains) > 0 {
		customSubjects := make([]string, len(customDomains))
		acmeDNSConfig := map[string]any{}

		for i, cd := range customDomains {
			wildcardDomain := "*." + cd.Domain
			customSubjects[i] = wildcardDomain
			acmeDNSConfig[wildcardDomain] = map[string]any{
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
					"email":  "admin@llmvault.dev",
					"challenges": map[string]any{
						"dns": map[string]any{
							"provider": map[string]any{
								"name":   "acmedns",
								"config": acmeDNSConfig,
							},
						},
					},
				},
			},
		})
	}

	// --- Certificates to automate ---
	automate := make([]string, len(customDomains))
	for i, cd := range customDomains {
		automate[i] = "*." + cd.Domain
	}

	// --- Full config ---
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

	// Add certificates.automate if there are custom domains
	if len(automate) > 0 {
		config["apps"].(map[string]any)["tls"].(map[string]any)["certificates"] = map[string]any{
			"automate": automate,
		}
	}

	return config
}

func (h *CustomDomainHandler) authProxyRoute(host, upstream string) map[string]any {
	return map[string]any{
		"match": []any{map[string]any{"host": []string{host}}},
		"handle": []any{
			map[string]any{
				"handler": "subroute",
				"routes": []any{
					map[string]any{
						"match": []any{map[string]any{"header": map[string][]string{"X-Internal-Secret": {"{env.LLMVAULT_INTERNAL_SECRET}"}}}},
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
				"X-Real-Ip":          {"{http.request.remote.host}"},
				"X-Forwarded-Proto":  {"{http.request.scheme}"},
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
		"match": []any{map[string]any{"host": []string{"dex.daytona.llmvault.dev"}}},
		"handle": []any{
			map[string]any{
				"handler": "subroute",
				"routes": []any{
					map[string]any{
						"handle": []any{map[string]any{
							"handler": "headers",
							"response": map[string]any{
								"set": map[string][]string{
									"Access-Control-Allow-Origin":  {"https://api.daytona.llmvault.dev"},
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
						"X-Forwarded-Host":              {"{http.request.host}"},
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
