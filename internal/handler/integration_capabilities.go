package handler

import "github.com/usehiveloop/hiveloop/internal/mcp/catalog"

func integrationEmployeeProfileCapability(provider string) *catalog.EmployeeProfileCapability {
	return catalog.Global().EmployeeProfileCapability(provider)
}
