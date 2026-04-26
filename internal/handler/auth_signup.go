package handler

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// createUserDefaultOrg is the single source of truth for "this user is signing
// up — give them their default workspace." All three signup entry points
// (password Register, OTPVerify, OAuth findOrCreateUser) call this so that any
// onboarding side effects (notably the welcome credit grant) live in one place
// and stay consistent.
//
// Side effects, all atomic with the caller's transaction:
//  1. Create the org named "<user.Name>'s Workspace" (PlanSlug defaults to "free").
//  2. Insert an owner OrgMembership for user → org.
//  3. If a free plan row exists with WelcomeCredits > 0, grant that amount to
//     the new org as a permanent (non-expiring) credit, refType="signup",
//     refID=user.ID. The unique credit-ledger index keys off (org, reason,
//     ref_type, ref_id), so the grant is naturally idempotent if signup is
//     ever retried for the same user.
//
// Welcome credits are intentionally only granted here. Subsequent orgs the
// user creates via /v1/orgs do not receive them — that handler stays
// untouched and goes through its own (un-helped) path.
func createUserDefaultOrg(tx *gorm.DB, credits *billing.CreditsService, user *model.User) (model.Org, error) {
	org := model.Org{
		Name: fmt.Sprintf("%s's Workspace", user.Name),
	}
	if err := tx.Create(&org).Error; err != nil {
		return org, fmt.Errorf("creating org: %w", err)
	}

	membership := model.OrgMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   "owner",
	}
	if err := tx.Create(&membership).Error; err != nil {
		return org, fmt.Errorf("creating membership: %w", err)
	}

	if err := grantWelcomeCredits(tx, credits, org.ID, user.ID); err != nil {
		return org, err
	}
	return org, nil
}

// grantWelcomeCredits looks up the free plan and, if its WelcomeCredits is
// configured (> 0), writes the grant entry on the supplied transaction.
//
// A missing free plan row is treated as "welcome grants are not configured"
// and is not an error — self-hosted deployments without a seeded plan catalog
// still complete signup successfully.
func grantWelcomeCredits(tx *gorm.DB, credits *billing.CreditsService, orgID, userID uuid.UUID) error {
	if credits == nil {
		return nil
	}

	var freePlan model.Plan
	err := tx.Where("slug = ?", billing.FreePlanSlug).First(&freePlan).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("loading free plan: %w", err)
	}

	if freePlan.WelcomeCredits <= 0 {
		return nil
	}

	if err := billing.GrantWithTx(
		tx,
		orgID,
		freePlan.WelcomeCredits,
		billing.ReasonWelcomeGrant,
		billing.RefTypeSignup,
		userID.String(),
		nil, // permanent — welcome credits do not expire
	); err != nil {
		return fmt.Errorf("granting welcome credits: %w", err)
	}
	return nil
}
