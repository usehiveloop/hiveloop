package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"gorm.io/gorm"

	polargo "github.com/polarsource/polar-go"
	"github.com/polarsource/polar-go/models/components"
	"github.com/polarsource/polar-go/models/operations"

	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// BillingHandler manages checkout, subscription status, and customer portal.
type BillingHandler struct {
	db     *gorm.DB
	polar  *polargo.Polar
	config *config.Config
}

// NewBillingHandler creates a billing handler.
func NewBillingHandler(db *gorm.DB, polar *polargo.Polar, cfg *config.Config) *BillingHandler {
	return &BillingHandler{
		db:     db,
		polar:  polar,
		config: cfg,
	}
}

// createCheckoutRequest is the request body for POST /v1/billing/checkout.
type createCheckoutRequest struct {
	ProductType string `json:"product_type"` // "pro_shared" or "pro_dedicated"
	SuccessURL  string `json:"success_url"`
}

type createCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

// CreateCheckout creates a Polar checkout session for the org.
func (handler *BillingHandler) CreateCheckout(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var body createCheckoutRequest
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	productID := handler.resolveProductID(body.ProductType)
	if productID == "" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid product_type"})
		return
	}

	ctx := request.Context()

	// Ensure org has a Polar customer
	polarCustomerID, err := handler.ensurePolarCustomer(ctx, org)
	if err != nil {
		slog.Error("billing: failed to ensure polar customer", "org_id", org.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create billing customer"})
		return
	}

	// Create checkout session
	checkoutResp, err := handler.polar.Checkouts.Create(ctx, components.CheckoutCreate{
		Products:   []string{productID},
		CustomerID: &polarCustomerID,
		SuccessURL: &body.SuccessURL,
		Metadata: map[string]components.CheckoutCreateMetadata{
			"org_id":       components.CreateCheckoutCreateMetadataStr(org.ID.String()),
			"product_type": components.CreateCheckoutCreateMetadataStr(body.ProductType),
		},
	})
	if err != nil {
		slog.Error("billing: failed to create checkout", "org_id", org.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
		return
	}

	slog.Info("billing: checkout created", "org_id", org.ID, "checkout_id", checkoutResp.Checkout.ID)

	writeJSON(writer, http.StatusOK, createCheckoutResponse{
		CheckoutURL: checkoutResp.Checkout.URL,
	})
}

type subscriptionResponse struct {
	Plan        string  `json:"plan"`
	Status      string  `json:"status"`
	ProductType *string `json:"product_type,omitempty"`
}

// GetSubscription returns the org's current subscription status.
func (handler *BillingHandler) GetSubscription(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var subscription model.Subscription
	err := handler.db.Where("org_id = ? AND status = 'active'", org.ID).
		Order("created_at DESC").
		First(&subscription).Error

	if err == gorm.ErrRecordNotFound {
		writeJSON(writer, http.StatusOK, subscriptionResponse{
			Plan:   "free",
			Status: "active",
		})
		return
	}
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to load subscription"})
		return
	}

	writeJSON(writer, http.StatusOK, subscriptionResponse{
		Plan:        "pro",
		Status:      subscription.Status,
		ProductType: &subscription.ProductType,
	})
}

type portalResponse struct {
	PortalURL string `json:"portal_url"`
}

// CreatePortal creates a Polar customer portal session.
func (handler *BillingHandler) CreatePortal(writer http.ResponseWriter, request *http.Request) {
	org, ok := middleware.OrgFromContext(request.Context())
	if !ok {
		writeJSON(writer, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	if org.PolarCustomerID == nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "no billing account found"})
		return
	}

	ctx := request.Context()

	sessionResp, err := handler.polar.CustomerSessions.Create(ctx,
		operations.CreateCustomerSessionsCreateCustomerSessionCreateCustomerSessionCustomerIDCreate(
			components.CustomerSessionCustomerIDCreate{
				CustomerID: *org.PolarCustomerID,
			},
		),
	)
	if err != nil {
		slog.Error("billing: failed to create portal session", "org_id", org.ID, "error", err)
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "failed to create portal session"})
		return
	}

	writeJSON(writer, http.StatusOK, portalResponse{
		PortalURL: sessionResp.CustomerSession.CustomerPortalURL,
	})
}

// ensurePolarCustomer creates a Polar customer for the org if one doesn't exist.
func (handler *BillingHandler) ensurePolarCustomer(ctx context.Context, org *model.Org) (string, error) {
	if org.PolarCustomerID != nil {
		return *org.PolarCustomerID, nil
	}

	// Look up the org owner's email for the customer record
	var membership model.OrgMembership
	if err := handler.db.Where("org_id = ?", org.ID).Order("created_at ASC").First(&membership).Error; err != nil {
		return "", err
	}
	var user model.User
	if err := handler.db.Where("id = ?", membership.UserID).First(&user).Error; err != nil {
		return "", err
	}

	externalID := org.ID.String()
	customerResp, err := handler.polar.Customers.Create(ctx,
		components.CreateCustomerCreateCustomerIndividualCreate(
			components.CustomerIndividualCreate{
				Email:      user.Email,
				Name:       &org.Name,
				ExternalID: &externalID,
			},
		),
	)
	if err != nil {
		return "", err
	}

	individual := customerResp.GetCustomerIndividual()
	if individual == nil {
		return "", fmt.Errorf("unexpected customer type returned from Polar")
	}
	customerID := individual.ID
	handler.db.Model(org).Update("polar_customer_id", customerID)
	org.PolarCustomerID = &customerID

	slog.Info("billing: polar customer created", "org_id", org.ID, "customer_id", customerID)

	return customerID, nil
}

// resolveProductID maps a product type to the configured Polar product ID.
func (handler *BillingHandler) resolveProductID(productType string) string {
	switch productType {
	case "pro_shared":
		return handler.config.PolarProductProSharedID
	case "pro_dedicated":
		return handler.config.PolarProductProDedicatedID
	default:
		return ""
	}
}
