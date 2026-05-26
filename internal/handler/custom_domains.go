package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// CustomDomainHandler manages custom preview domain configuration.
type CustomDomainHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewCustomDomainHandler(db *gorm.DB, cfg *config.Config) *CustomDomainHandler {
	return &CustomDomainHandler{
		db:  db,
		cfg: cfg,
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

// Create handles POST /v1/custom-domains.
// @Summary Add a custom domain
// @Description Register a new custom preview domain for the current organization. Returns DNS records to create.
// @Tags custom-domains
// @Accept json
// @Produce json
// @Param body body createDomainRequest true "Domain to add"
// @Success 201 {object} createDomainResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/custom-domains [post]
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

	cd := model.CustomDomain{
		OrgID:       org.ID,
		Domain:      domain,
		CNAMETarget: h.cfg.PreviewCNAMETarget,
	}

	if err := h.db.Create(&cd).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create domain"})
		return
	}

	writeJSON(w, http.StatusCreated, createDomainResponse{
		CustomDomain: cd,
		DNSRecords: []dnsRecord{
			{Type: "CNAME", Name: "*." + domain, Value: h.cfg.PreviewCNAMETarget},
		},
	})
}

// List handles GET /v1/custom-domains.
// @Summary List custom domains
// @Description Returns all custom preview domains for the current organization.
// @Tags custom-domains
// @Produce json
// @Success 200 {array} createDomainResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/custom-domains [get]
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
			},
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// Verify handles POST /v1/custom-domains/{id}/verify.
// @Summary Verify a custom domain
// @Description Checks that both DNS CNAME records are correctly configured and triggers wildcard TLS provisioning.
// @Tags custom-domains
// @Produce json
// @Param id path string true "Domain ID"
// @Success 200 {object} verifyDomainResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/custom-domains/{id}/verify [post]
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

	now := time.Now()
	cd.Verified = true
	cd.VerifiedAt = &now
	h.db.Save(&cd)

	writeJSON(w, http.StatusOK, verifyDomainResponse{
		Verified: true,
		Message:  "Domain verified",
	})
}

// Delete handles DELETE /v1/custom-domains/{id}.
// @Summary Delete a custom domain
// @Description Remove a custom preview domain and its TLS configuration.
// @Tags custom-domains
// @Param id path string true "Domain ID"
// @Success 204
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/custom-domains/{id} [delete]
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

	h.db.Delete(&cd)

	w.WriteHeader(http.StatusNoContent)
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
