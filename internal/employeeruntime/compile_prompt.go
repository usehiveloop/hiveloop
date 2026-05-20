package employeeruntime

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/model"
)

func buildPromptFragments(ctx context.Context, db *gorm.DB, agent *model.Agent, description string) PromptFragments {
	var org model.Org
	var hasOrg bool
	if agent.OrgID != nil && db != nil {
		if err := db.WithContext(ctx).Where("id = ?", *agent.OrgID).First(&org).Error; err == nil {
			hasOrg = true
		}
	}

	fragments := PromptFragments{
		Identity: PromptFragment{
			Title: "Your identity",
			Content: strings.TrimSpace(strings.Join([]string{
				identityOpening(org, hasOrg),
				"Name: " + managedEmployeeName,
				optionalLine("Role description", description),
				employeeIdentityPrompt(agent),
			}, "\n")),
		},
	}
	if hasOrg {
		companyContent := strings.TrimSpace(org.PromptCompany)
		if companyContent == "" {
			companyContent = defaultCompanyPrompt(org)
		}
		if companyContent != "" {
			fragments.Company = PromptFragment{Title: "About the company", Content: companyContent}
		}
	}
	return fragments
}

func identityOpening(org model.Org, hasOrg bool) string {
	companyName := "this company"
	if hasOrg && strings.TrimSpace(org.Name) != "" {
		companyName = strings.TrimSpace(org.Name)
	}
	return fmt.Sprintf("You are a %s employee.", companyName)
}

func employeeIdentityPrompt(agent *model.Agent) string {
	return employeeprompts.EngineeringIdentityPrompt
}

func isDefaultManagedEmployeeIdentityPrompt(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	return prompt == "" ||
		prompt == strings.TrimSpace(employeeprompts.EngineeringIdentityPrompt) ||
		prompt == strings.TrimSpace(employeeprompts.LegacyEngineeringIdentityPromptV1)
}

func optionalLine(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func defaultCompanyPrompt(org model.Org) string {
	var parts []string
	if org.Name != "" {
		parts = append(parts, "Company name: "+org.Name)
	}
	if org.Website != "" {
		parts = append(parts, "Website: "+org.Website)
	}
	if org.Description != "" {
		parts = append(parts, "Company description: "+org.Description)
	}
	return strings.Join(parts, "\n")
}
