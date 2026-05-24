package registry

import (
	"fmt"
	"sort"
)

type ModelRoute struct {
	ProviderID string
	ModelID    string
}

type RoutedModel struct {
	Model
	ProviderIDs []string
}

type ResolvedModelRoute struct {
	CanonicalID string
	ProviderID  string
	UpstreamID  string
	Model       Model
}

var explicitModelRoutes = map[string][]ModelRoute{
	"claude-sonnet-4.6": {
		{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"},
	},
	"gemini-3-flash-preview": {
		{ProviderID: "google", ModelID: "gemini-3-flash-preview"},
		{ProviderID: "openrouter", ModelID: "google/gemini-3-flash-preview"},
	},
	"deepseek-v4-flash": {
		{ProviderID: "crof", ModelID: "deepseek-v4-flash"},
		{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-flash"},
	},
	"deepseek-v4-pro": {
		{ProviderID: "crof", ModelID: "deepseek-v4-pro"},
		{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
	},
	"kimi-k2.5": {
		{ProviderID: "crof", ModelID: "kimi-k2.5"},
		{ProviderID: "moonshotai", ModelID: "kimi-k2.5"},
		{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.5"},
	},
}

func (r *Registry) ResolveModel(providerID, canonicalID string) (ResolvedModelRoute, bool) {
	if routes, ok := explicitModelRoutes[canonicalID]; ok {
		for _, route := range routes {
			if route.ProviderID != providerID {
				continue
			}
			mdl, ok := r.providerModel(route.ProviderID, route.ModelID)
			if !ok {
				return ResolvedModelRoute{}, false
			}
			mdl.ID = canonicalID
			mdl.Hidden = false
			return ResolvedModelRoute{
				CanonicalID: canonicalID,
				ProviderID:  route.ProviderID,
				UpstreamID:  route.ModelID,
				Model:       mdl,
			}, true
		}
		return ResolvedModelRoute{}, false
	}

	mdl, ok := r.providerModel(providerID, canonicalID)
	if !ok || mdl.Hidden {
		return ResolvedModelRoute{}, false
	}
	return ResolvedModelRoute{
		CanonicalID: canonicalID,
		ProviderID:  providerID,
		UpstreamID:  canonicalID,
		Model:       mdl,
	}, true
}

func (r *Registry) CanonicalModel(canonicalID string) (RoutedModel, bool) {
	if routes, ok := explicitModelRoutes[canonicalID]; ok {
		for _, route := range routes {
			mdl, ok := r.providerModel(route.ProviderID, route.ModelID)
			if !ok {
				continue
			}
			mdl.ID = canonicalID
			mdl.Hidden = false
			return RoutedModel{Model: mdl, ProviderIDs: providerIDsForRoutes(routes)}, true
		}
		return RoutedModel{}, false
	}

	providerIDs := []string{}
	var out Model
	for _, provider := range r.AllProviders() {
		mdl, ok := provider.Models[canonicalID]
		if !ok || mdl.Hidden {
			continue
		}
		if isExplicitRouteUpstream(provider.ID, canonicalID) {
			continue
		}
		if len(providerIDs) == 0 {
			out = mdl
		}
		providerIDs = append(providerIDs, provider.ID)
	}
	if len(providerIDs) == 0 {
		return RoutedModel{}, false
	}
	sort.Strings(providerIDs)
	return RoutedModel{Model: out, ProviderIDs: providerIDs}, true
}

func (r *Registry) CanonicalModelsForProviders(providerIDs []string) []RoutedModel {
	allowed := map[string]bool{}
	for _, providerID := range providerIDs {
		allowed[providerID] = true
	}

	byID := map[string]RoutedModel{}
	for canonicalID, routes := range explicitModelRoutes {
		for _, route := range routes {
			if !allowed[route.ProviderID] {
				continue
			}
			resolved, ok := r.ResolveModel(route.ProviderID, canonicalID)
			if !ok {
				continue
			}
			rm := byID[canonicalID]
			if rm.ID == "" {
				rm.Model = resolved.Model
				rm.ProviderIDs = []string{}
			}
			rm.ProviderIDs = appendIfMissing(rm.ProviderIDs, route.ProviderID)
			byID[canonicalID] = rm
		}
	}

	for providerID := range allowed {
		provider, ok := r.GetProvider(providerID)
		if !ok {
			continue
		}
		for _, mdl := range provider.Models {
			if mdl.Hidden {
				continue
			}
			if _, explicit := explicitModelRoutes[mdl.ID]; explicit {
				continue
			}
			if isExplicitRouteUpstream(providerID, mdl.ID) {
				continue
			}
			rm := byID[mdl.ID]
			if rm.ID == "" {
				rm.Model = mdl
				rm.ProviderIDs = []string{}
			}
			rm.ProviderIDs = appendIfMissing(rm.ProviderIDs, providerID)
			byID[mdl.ID] = rm
		}
	}

	out := make([]RoutedModel, 0, len(byID))
	for _, rm := range byID {
		sort.Strings(rm.ProviderIDs)
		out = append(out, rm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Registry) ValidateCanonicalModel(canonicalID string) error {
	if canonicalID == "" {
		return nil
	}
	if _, ok := r.CanonicalModel(canonicalID); ok {
		return nil
	}
	return fmt.Errorf("model %q is not in the catalog", canonicalID)
}

func (r *Registry) providerModel(providerID, modelID string) (Model, bool) {
	provider, ok := r.GetProvider(providerID)
	if !ok {
		return Model{}, false
	}
	mdl, ok := provider.Models[modelID]
	return mdl, ok
}

func providerIDsForRoutes(routes []ModelRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = appendIfMissing(out, route.ProviderID)
	}
	sort.Strings(out)
	return out
}

func isExplicitRouteUpstream(providerID, modelID string) bool {
	for _, routes := range explicitModelRoutes {
		for _, route := range routes {
			if route.ProviderID == providerID && route.ModelID == modelID {
				return true
			}
		}
	}
	return false
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
