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

func (r *Registry) ResolveModel(providerID, canonicalID string) (ResolvedModelRoute, bool) {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return ResolvedModelRoute{}, false
	}
	for _, route := range hivyModel.Routes {
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

func (r *Registry) CanonicalModel(canonicalID string) (RoutedModel, bool) {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return RoutedModel{}, false
	}
	for _, route := range hivyModel.Routes {
		mdl, ok := r.providerModel(route.ProviderID, route.ModelID)
		if !ok {
			continue
		}
		mdl.ID = canonicalID
		mdl.Hidden = false
		return RoutedModel{Model: mdl, ProviderIDs: providerIDsForRoutes(hivyModel.Routes)}, true
	}
	return RoutedModel{}, false
}

func (r *Registry) ProviderPreferenceForModel(canonicalID string) []string {
	hivyModel, ok := hivyModelsByID[canonicalID]
	if !ok {
		return nil
	}
	providerIDs := make([]string, 0, len(hivyModel.Routes))
	for _, route := range hivyModel.Routes {
		if _, ok := r.providerModel(route.ProviderID, route.ModelID); !ok {
			continue
		}
		providerIDs = appendIfMissing(providerIDs, route.ProviderID)
	}
	return providerIDs
}

func (r *Registry) CanonicalModelsForProviders(providerIDs []string) []RoutedModel {
	allowed := map[string]bool{}
	for _, providerID := range providerIDs {
		allowed[providerID] = true
	}

	byID := map[string]RoutedModel{}
	for _, hivyModel := range supportedHivyModels {
		for _, route := range hivyModel.Routes {
			if !allowed[route.ProviderID] {
				continue
			}
			resolved, ok := r.ResolveModel(route.ProviderID, hivyModel.ID)
			if !ok {
				continue
			}
			rm := byID[hivyModel.ID]
			if rm.ID == "" {
				rm.Model = resolved.Model
				rm.ProviderIDs = []string{}
			}
			rm.ProviderIDs = appendIfMissing(rm.ProviderIDs, route.ProviderID)
			byID[hivyModel.ID] = rm
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

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
