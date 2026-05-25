package hindsight

import "github.com/google/uuid"

// OrgBankID returns the Hindsight bank ID for an org.
func OrgBankID(orgID uuid.UUID) string {
	return "org-" + orgID.String()
}
