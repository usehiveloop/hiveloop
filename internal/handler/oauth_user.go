package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// errOAuthLinkRequiresConfirmation is returned by findOrCreateUser when an
// OAuth sign-in matches a confirmed local account that has not yet linked the
// provider. The caller must surface an error asking the user to log in with
// their existing credentials and link the provider from their account
// settings; we never silently merge the sessions.
var errOAuthLinkRequiresConfirmation = errors.New("oauth: account exists; link required")

// oauthProfile holds the normalised user info fetched from an OAuth provider.

// provider (e.g. X/Twitter) does not return a user email.

// isPlaceholderEmail reports whether the email is a generated placeholder.
func (h *OAuthHandler) issueTokensAndRespond(w http.ResponseWriter, status int, user model.User, orgID, role string, memberships []model.OrgMembership) {
	accessToken, err := auth.IssueAccessToken(h.privateKey, h.issuer, h.audience, user.ID.String(), orgID, role, h.accessTTL)
	if err != nil {
		slog.Error("failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshToken(h.signingKey, user.ID.String(), h.refreshTTL)
	if err != nil {
		slog.Error("failed to issue refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Store refresh token hash for revocation tracking.
	sum := sha256.Sum256([]byte(refreshToken))
	storedRefresh := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&storedRefresh).Error; err != nil {
		slog.Error("failed to store refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, m := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   m.OrgID.String(),
			Name: m.Org.Name,
			Role: m.Role,
		})
	}

	writeJSON(w, status, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(h.accessTTL.Seconds()),
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}

// ---------------------------------------------------------------------------
// Account linking
// ---------------------------------------------------------------------------

func (h *OAuthHandler) findOrCreateUser(provider string, profile *oauthProfile) (*model.User, error) {
	// 1. Check if this OAuth account already exists.
	var existing model.OAuthAccount
	err := h.db.Where("provider = ? AND provider_user_id = ?", provider, profile.ProviderUserID).First(&existing).Error
	if err == nil {
		// Returning user — just load them.
		var user model.User
		if err := h.db.Where("id = ?", existing.UserID).First(&user).Error; err != nil {
			return nil, fmt.Errorf("loading linked user: %w", err)
		}
		return &user, nil
	}

	// 2. No existing link — check if a user with this email exists.
	email := strings.ToLower(strings.TrimSpace(profile.Email))
	var user model.User
	err = h.db.Where("email = ?", email).First(&user).Error

	if err == nil {
		// A local account with this email already exists. Silently linking a
		// new OAuth provider to a confirmed account is a takeover risk: an
		// attacker who controls an OAuth-verified email could claim a
		// pre-existing local account. Require an explicit, authenticated
		// link step. Unconfirmed accounts (which never proved email
		// ownership) are still auto-linked since they carry no trust.
		if user.EmailConfirmedAt != nil {
			return nil, errOAuthLinkRequiresConfirmation
		}

		oauthAcct := model.OAuthAccount{
			UserID:         user.ID,
			Provider:       provider,
			ProviderUserID: profile.ProviderUserID,
		}
		if err := h.db.Create(&oauthAcct).Error; err != nil {
			return nil, fmt.Errorf("linking oauth account: %w", err)
		}
		// The OAuth provider just verified this email; promote the account
		// to confirmed unless the address is a placeholder (e.g. X/Twitter).
		if !isPlaceholderEmail(email) {
			now := time.Now()
			h.db.Model(&user).Update("email_confirmed_at", &now)
			user.EmailConfirmedAt = &now
		}
		return &user, nil
	}

	// 3. Brand new user — create everything in a transaction.
	now := time.Now()
	name := profile.Name
	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	var emailConfirmedAt *time.Time
	if !isPlaceholderEmail(email) {
		emailConfirmedAt = &now
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{
			Email:            email,
			Name:             name,
			EmailConfirmedAt: emailConfirmedAt,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating user: %w", err)
		}

		org := model.Org{
			Name: fmt.Sprintf("%s's Workspace", name),
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership := model.OrgMembership{
			UserID: user.ID,
			OrgID:  org.ID,
			Role:   "owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		oauthAcct := model.OAuthAccount{
			UserID:         user.ID,
			Provider:       provider,
			ProviderUserID: profile.ProviderUserID,
		}
		if err := tx.Create(&oauthAcct).Error; err != nil {
			return fmt.Errorf("creating oauth account: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// ---------------------------------------------------------------------------
// Provider profile fetchers
// ---------------------------------------------------------------------------

func (h *OAuthHandler) fetchProfile(ctx context.Context, provider string, token *oauth2.Token) (*oauthProfile, error) {
	switch provider {
	case "github":
		return fetchGitHubProfile(ctx, token)
	case "google":
		return fetchGoogleProfile(ctx, token)
	case "x":
		return fetchXProfile(ctx, token)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

