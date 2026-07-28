package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/Lore-Hex/AllRouter/internal/config"
)

const (
	backupConfigPath   = "/config/backups"
	maxConfigBodyBytes = 64 << 10
)

type runtimeSettings struct {
	once         sync.Once
	mu           sync.RWMutex
	backupModels []string
}

type backupConfigResponse struct {
	Preset             config.Preset  `json:"preset"`
	PrimaryModelSource string         `json:"primary_model_source"`
	BackupModels       []string       `json:"backup_models"`
	AvailableModels    []string       `json:"available_models"`
	Models             []modelOption  `json:"models"`
	Profiles           []routeProfile `json:"profiles"`
	MaxBackupModels    int            `json:"max_backup_models"`
	PersistenceEnabled bool           `json:"persistence_enabled"`
	CatalogError       string         `json:"catalog_error,omitempty"`
}

type modelOption struct {
	ID                                string `json:"id"`
	Name                              string `json:"name"`
	Provider                          string `json:"provider"`
	ContextLength                     int64  `json:"context_length"`
	InputPriceMicrodollarsPerMillion  int64  `json:"input_price_microdollars_per_million"`
	OutputPriceMicrodollarsPerMillion int64  `json:"output_price_microdollars_per_million"`
	PrivacyTier                       int    `json:"privacy_tier"`
	PrivacyLabel                      string `json:"privacy_label"`
	RouteKind                         string `json:"route_kind"`
	OpenWeights                       bool   `json:"open_weights"`
	ProviderCount                     int    `json:"provider_count"`
	Managed                           bool   `json:"managed"`
}

type routeProfile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	PrimaryModel string   `json:"primary_model"`
	SmallModel   string   `json:"small_model"`
	BackupModels []string `json:"backup_models"`
}

type backupConfigUpdate struct {
	BackupModels []string `json:"backup_models"`
}

func (s *Server) initializeRuntimeSettings() {
	s.runtime.once.Do(func() {
		s.runtime.backupModels = append([]string(nil), s.cfg.BackupModels...)
	})
}

func (s *Server) currentBackupModels() []string {
	s.initializeRuntimeSettings()
	s.runtime.mu.RLock()
	defer s.runtime.mu.RUnlock()
	return append([]string(nil), s.runtime.backupModels...)
}

func (s *Server) replaceBackupModels(models []string) error {
	normalized, err := config.NormalizeBackupModels(models)
	if err != nil {
		return err
	}
	s.initializeRuntimeSettings()
	s.runtime.mu.Lock()
	defer s.runtime.mu.Unlock()
	if err := config.SaveRuntimeConfig(s.cfg.ConfigFile, normalized); err != nil {
		return err
	}
	s.runtime.backupModels = append([]string(nil), normalized...)
	return nil
}

func (s *Server) handleBackupConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		s.writeBackupConfig(w, r)
	case http.MethodPut:
		s.updateBackupConfig(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", "invalid_request_error")
	}
}

func (s *Server) writeBackupConfig(w http.ResponseWriter, r *http.Request) {
	models, err := s.trustedRouterModelOptions(r)
	payload := s.backupConfigPayload(models)
	if err != nil {
		payload.CatalogError = err.Error()
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) updateBackupConfig(w http.ResponseWriter, r *http.Request) {
	if s.tr == nil {
		writeError(w, http.StatusConflict, "trustedrouter_not_configured", "configure TrustedRouter before adding backup models", "invalid_request_error")
		return
	}
	var update backupConfigUpdate
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConfigBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error(), "invalid_request_error")
		return
	}
	if err := ensureRequestJSONEOF(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error(), "invalid_request_error")
		return
	}
	normalized, err := config.NormalizeBackupModels(update.BackupModels)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_backup_models", err.Error(), "invalid_request_error")
		return
	}
	if s.cfg.Preset == config.PresetBackupRouter && len(normalized) == 0 {
		writeError(w, http.StatusBadRequest, "backup_models_required", "BackupRouter requires at least one backup model", "invalid_request_error")
		return
	}
	options, err := s.trustedRouterModelOptions(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "catalog_unavailable", "could not validate models against the TrustedRouter catalog", "api_error")
		return
	}
	known := make(map[string]struct{}, len(options))
	for _, model := range options {
		known[model.ID] = struct{}{}
	}
	for _, model := range normalized {
		if _, ok := known[model]; !ok {
			writeError(w, http.StatusBadRequest, "unknown_model", fmt.Sprintf("model %q is not a current TrustedRouter chat model", model), "invalid_request_error")
			return
		}
	}
	if err := s.replaceBackupModels(normalized); err != nil {
		writeError(w, http.StatusInternalServerError, "config_persistence_failed", "could not persist BackupRouter configuration", "api_error")
		return
	}
	writeJSON(w, http.StatusOK, s.backupConfigPayload(options))
}

func (s *Server) backupConfigPayload(models []modelOption) backupConfigResponse {
	available := make([]string, 0, len(models))
	for _, model := range models {
		available = append(available, model.ID)
	}
	return backupConfigResponse{
		Preset:             s.cfg.Preset,
		PrimaryModelSource: "Claude Code request",
		BackupModels:       s.currentBackupModels(),
		AvailableModels:    append([]string(nil), available...),
		Models:             append([]modelOption(nil), models...),
		Profiles:           recommendedRouteProfiles(available),
		MaxBackupModels:    config.MaxBackupModels,
		PersistenceEnabled: strings.TrimSpace(s.cfg.ConfigFile) != "",
	}
}

func (s *Server) trustedRouterModelOptions(r *http.Request) ([]modelOption, error) {
	if s.tr == nil {
		return []modelOption{}, nil
	}
	models, err := s.cachedTrustedRouterModels(r.Context())
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(models))
	options := make([]modelOption, 0, len(models))
	for _, model := range models {
		option, ok := catalogModelOption(model)
		if !ok {
			continue
		}
		if _, exists := seen[option.ID]; exists {
			continue
		}
		seen[option.ID] = struct{}{}
		options = append(options, option)
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].ID < options[j].ID
	})
	return options, nil
}

func catalogModelOption(model map[string]any) (modelOption, bool) {
	id, _ := model["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(strings.ToLower(id), "local/") {
		return modelOption{}, false
	}
	details, _ := model["trustedrouter"].(map[string]any)
	if boolCatalogField(details, "internal_only") || boolCatalogField(details, "configuration_hidden") {
		return modelOption{}, false
	}
	if supportsChat, exists := details["supports_chat"].(bool); exists && !supportsChat {
		return modelOption{}, false
	}
	architecture, _ := model["architecture"].(map[string]any)
	modality, _ := architecture["modality"].(string)
	if strings.Contains(strings.ToLower(modality), "embedding") {
		return modelOption{}, false
	}

	name, _ := model["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = id
	}
	provider, _ := details["provider"].(string)
	if strings.TrimSpace(provider) == "" {
		provider = strings.SplitN(id, "/", 2)[0]
	}
	privacyLabel, _ := details["privacy_tier_label"].(string)
	routeKind, _ := details["route_kind"].(string)
	return modelOption{
		ID:                                id,
		Name:                              strings.TrimSpace(name),
		Provider:                          strings.TrimSpace(provider),
		ContextLength:                     catalogInt64(model, "context_length"),
		InputPriceMicrodollarsPerMillion:  catalogModelPrice(details, model, "prompt_price_microdollars_per_million_tokens", "prompt"),
		OutputPriceMicrodollarsPerMillion: catalogModelPrice(details, model, "completion_price_microdollars_per_million_tokens", "completion"),
		PrivacyTier:                       int(catalogInt64(details, "privacy_tier")),
		PrivacyLabel:                      strings.TrimSpace(privacyLabel),
		RouteKind:                         strings.TrimSpace(routeKind),
		OpenWeights:                       boolCatalogField(details, "open_weights"),
		ProviderCount:                     catalogProviderCount(details),
		Managed:                           routeKind != "" && routeKind != "model",
	}, true
}

func catalogModelPrice(details, model map[string]any, exactKey, tokenKey string) int64 {
	if exact := catalogInt64(details, exactKey); exact > 0 {
		return exact
	}
	pricing, _ := model["pricing"].(map[string]any)
	perToken, ok := numericField(pricing, tokenKey)
	if !ok || perToken <= 0 {
		return 0
	}
	return int64(math.Round(perToken * 1_000_000_000_000))
}

func catalogInt64(fields map[string]any, key string) int64 {
	value, ok := numericField(fields, key)
	if !ok || value <= 0 || value > math.MaxInt64 {
		return 0
	}
	return int64(math.Round(value))
}

func boolCatalogField(fields map[string]any, key string) bool {
	value, _ := fields[key].(bool)
	return value
}

func catalogProviderCount(details map[string]any) int {
	endpoints, _ := details["endpoints"].([]any)
	seen := make(map[string]struct{}, len(endpoints))
	for _, raw := range endpoints {
		endpoint, _ := raw.(map[string]any)
		provider, _ := endpoint["provider"].(string)
		provider = strings.TrimSpace(provider)
		if provider != "" {
			seen[provider] = struct{}{}
		}
	}
	return len(seen)
}

func recommendedRouteProfiles(available []string) []routeProfile {
	known := make(map[string]struct{}, len(available))
	for _, model := range available {
		known[model] = struct{}{}
	}
	specs := []routeProfile{
		{
			ID:           "balanced",
			Name:         "Balanced",
			Description:  "Adaptive quality with a fast small-task model and broad recovery.",
			PrimaryModel: "trustedrouter/auto",
			SmallModel:   "trustedrouter/fast",
			BackupModels: []string{"moonshotai/kimi-k3", "z-ai/glm-5.2"},
		},
		{
			ID:           "fast",
			Name:         "Fast",
			Description:  "Minimize waiting for interactive coding and background work.",
			PrimaryModel: "trustedrouter/fast",
			SmallModel:   "trustedrouter/fast",
			BackupModels: []string{"trustedrouter/auto", "deepseek/deepseek-v4-flash"},
		},
		{
			ID:           "economy",
			Name:         "Economy",
			Description:  "Use low-cost managed pools before escalating to larger models.",
			PrimaryModel: "trustedrouter/cheap",
			SmallModel:   "trustedrouter/cheap",
			BackupModels: []string{"openai/gpt-oss-120b", "deepseek/deepseek-v4-flash"},
		},
		{
			ID:           "zdr",
			Name:         "Zero retention",
			Description:  "Keep every configured route on providers with a zero-retention policy.",
			PrimaryModel: "trustedrouter/zdr",
			SmallModel:   "trustedrouter/zdr",
			BackupModels: []string{"trustedrouter/e2e"},
		},
		{
			ID:           "e2e",
			Name:         "End to end encrypted",
			Description:  "Keep model compute inside confidential-compute routes.",
			PrimaryModel: "trustedrouter/e2e",
			SmallModel:   "trustedrouter/e2e",
			BackupModels: []string{"trustedrouter/confidential"},
		},
	}
	profiles := make([]routeProfile, 0, len(specs))
	for _, profile := range specs {
		if _, ok := known[profile.PrimaryModel]; !ok {
			continue
		}
		if _, ok := known[profile.SmallModel]; !ok {
			profile.SmallModel = profile.PrimaryModel
		}
		filtered := make([]string, 0, len(profile.BackupModels))
		for _, model := range profile.BackupModels {
			if _, ok := known[model]; ok && model != profile.PrimaryModel {
				filtered = append(filtered, model)
			}
		}
		profile.BackupModels = filtered
		profiles = append(profiles, profile)
	}
	return profiles
}

func ensureRequestJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("request contains multiple JSON values")
}
