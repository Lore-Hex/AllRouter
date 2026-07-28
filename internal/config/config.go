package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	trustedrouter "github.com/Lore-Hex/trusted-router-go"
)

const (
	envListen              = "ALLROUTER_LISTEN"
	envLocalURL            = "ALLROUTER_LOCAL_URL"
	envTRAPIKey            = "TRUSTEDROUTER_API_KEY"
	envTRBaseURL           = "ALLROUTER_TR_BASE_URL"
	envTRCatalogURL        = "ALLROUTER_TR_CATALOG_URL"
	envLocalMaxConcurrency = "ALLROUTER_LOCAL_MAX_CONCURRENCY"
	envLocalQueueWait      = "ALLROUTER_LOCAL_QUEUE_WAIT"
	envLocalSlowAfter      = "ALLROUTER_LOCAL_SLOW_AFTER"
	envBurstOnError        = "ALLROUTER_BURST_ON_ERROR"
	envBurstFallbackModel  = "ALLROUTER_BURST_FALLBACK_MODEL"
	envPreset              = "ALLROUTER_PRESET"
	envBackupModels        = "ALLROUTER_BACKUP_MODELS"
	envToken               = "ALLROUTER_TOKEN"
	envAliases             = "ALLROUTER_ALIASES"
	envSavingsReference    = "ALLROUTER_SAVINGS_REFERENCE"
	envStateFile           = "ALLROUTER_STATE_FILE"
	envConfigFile          = "ALLROUTER_CONFIG_FILE"
	envCloud               = "ALLROUTER_CLOUD"
	envMaxCloudSpend       = "ALLROUTER_MAX_CLOUD_SPEND"
	envSSEBatchWindow      = "ALLROUTER_SSE_BATCH_WINDOW"
	envSSEBatchMaxBytes    = "ALLROUTER_SSE_BATCH_MAX_BYTES"
)

// DefaultTRCatalogURL is the public TrustedRouter control-plane catalog base URL.
const DefaultTRCatalogURL = "https://trustedrouter.com/v1"

// Preset enables a coherent routing policy without hiding its controls.
type Preset string

const (
	PresetNone         Preset = ""
	PresetBackupRouter Preset = "backuprouter"
)

// defaultBackupModels follow the Claude model requested by Claude Code when
// BackupRouter mode is enabled.
var defaultBackupModels = []string{
	"moonshotai/kimi-k3",
	"z-ai/glm-5.2",
}

// MaxBackupModels bounds the configured failover chain and its request size.
const MaxBackupModels = 16

// CloudMode controls when AllRouter may send traffic to the cloud upstream.
type CloudMode string

const (
	CloudAuto     CloudMode = "auto"
	CloudExplicit CloudMode = "explicit"
	CloudOff      CloudMode = "off"
)

// Config is the complete runtime configuration for an AllRouter process.
type Config struct {
	Listen               string
	LocalURL             string
	TRAPIKey             string
	TRBaseURL            string
	TRCatalogURL         string
	LocalMaxConcurrency  int
	LocalQueueWait       time.Duration
	LocalSlowAfter       time.Duration
	BurstOnError         bool
	BurstFallbackModel   string
	Preset               Preset
	BackupModels         []string
	Token                string
	Aliases              map[string]string
	SavingsReference     string
	StateFile            string
	ConfigFile           string
	Cloud                CloudMode
	MaxCloudSpendMicro   int64
	SSEBatchWindow       time.Duration
	SSEBatchMaxBytes     int
	NoAutodetect         bool
	PrintVersion         bool
	backupModelsExplicit bool
}

// HasLocal reports whether local upstream routing is configured.
func (c Config) HasLocal() bool {
	return strings.TrimSpace(c.LocalURL) != ""
}

// HasTrustedRouter reports whether TrustedRouter routing is configured.
func (c Config) HasTrustedRouter() bool {
	return strings.TrimSpace(c.TRAPIKey) != ""
}

// Parse parses flags with environment-variable fallbacks. Flag values win over
// environment values because env/default values are installed as flag defaults.
func Parse(args []string, lookupEnv func(string) (string, bool), output io.Writer) (Config, error) {
	cfg, err := defaultsFromEnv(lookupEnv)
	if err != nil {
		return Config{}, err
	}

	fs := flag.NewFlagSet("allrouter", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "bind address")
	fs.StringVar(&cfg.LocalURL, "local-url", cfg.LocalURL, "local OpenAI-compatible base URL")
	fs.StringVar(&cfg.TRAPIKey, "tr-api-key", cfg.TRAPIKey, "TrustedRouter API key")
	fs.StringVar(&cfg.TRBaseURL, "tr-base-url", cfg.TRBaseURL, "TrustedRouter OpenAI-compatible base URL")
	fs.StringVar(&cfg.TRCatalogURL, "tr-catalog-url", cfg.TRCatalogURL, "TrustedRouter public catalog base URL")
	fs.IntVar(&cfg.LocalMaxConcurrency, "local-max-concurrency", cfg.LocalMaxConcurrency, "in-flight cap on local upstream")
	fs.DurationVar(&cfg.LocalQueueWait, "local-queue-wait", cfg.LocalQueueWait, "how long to wait for a local slot before bursting")
	fs.DurationVar(&cfg.LocalSlowAfter, "local-slow-after", cfg.LocalSlowAfter, "burst when local response body does not produce its first byte within this duration; 0 disables")
	fs.BoolVar(&cfg.BurstOnError, "burst-on-error", cfg.BurstOnError, "burst to TrustedRouter on local connect error/timeout/429/5xx/404-model")
	fs.StringVar(&cfg.BurstFallbackModel, "burst-fallback-model", cfg.BurstFallbackModel, "TrustedRouter model to use when bursting an unmapped local-native model")
	fs.Var(presetValue{value: &cfg.Preset}, "preset", "routing preset: backuprouter")
	fs.Var(&modelListValue{values: &cfg.BackupModels, explicit: &cfg.backupModelsExplicit}, "backup-model", "TrustedRouter fallback model after the requested model; repeatable")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "optional inbound bearer token")
	fs.Var(aliasMapValue{values: cfg.Aliases}, "alias", "model alias CLOUD-id=LOCAL-model; repeatable")
	fs.StringVar(&cfg.SavingsReference, "savings-reference", cfg.SavingsReference, "TrustedRouter model id used as the counterfactual savings price when no alias/request price applies")
	fs.StringVar(&cfg.StateFile, "state-file", cfg.StateFile, "state file path; empty disables persistence")
	fs.StringVar(&cfg.ConfigFile, "config-file", cfg.ConfigFile, "runtime config file path; empty disables UI persistence")
	fs.Var(cloudModeValue{value: &cfg.Cloud}, "cloud", "cloud egress mode: auto, explicit, or off")
	fs.Var(usdMicroValue{value: &cfg.MaxCloudSpendMicro}, "max-cloud-spend", "maximum cloud spend in USD per UTC day; 0 disables the cap")
	fs.DurationVar(&cfg.SSEBatchWindow, "sse-batch-window", cfg.SSEBatchWindow, "coalesce streamed chat SSE content chunks within this window to cut egress bytes (useful when exposed over ngrok/WAN); 0 disables")
	fs.IntVar(&cfg.SSEBatchMaxBytes, "sse-batch-max-bytes", cfg.SSEBatchMaxBytes, "max buffered content bytes before a coalesced SSE chunk is flushed")
	fs.BoolVar(&cfg.NoAutodetect, "no-autodetect", cfg.NoAutodetect, "disable local server autodetection when -local-url is unset")
	fs.BoolVar(&cfg.PrintVersion, "version", cfg.PrintVersion, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage: allrouter [flags]")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Flags:")
		fmt.Fprintln(output, "  -listen                    env ALLROUTER_LISTEN                  default :8383")
		fmt.Fprintln(output, "  -local-url                 env ALLROUTER_LOCAL_URL               default \"\"")
		fmt.Fprintln(output, "  -tr-api-key                env TRUSTEDROUTER_API_KEY          default \"\"")
		fmt.Fprintf(output, "  -tr-base-url               env ALLROUTER_TR_BASE_URL             default %s\n", trustedrouter.DefaultAPIBaseURL)
		fmt.Fprintf(output, "  -tr-catalog-url            env ALLROUTER_TR_CATALOG_URL          default %s\n", DefaultTRCatalogURL)
		fmt.Fprintln(output, "  -local-max-concurrency     env ALLROUTER_LOCAL_MAX_CONCURRENCY   default 4")
		fmt.Fprintln(output, "  -local-queue-wait          env ALLROUTER_LOCAL_QUEUE_WAIT        default 0s")
		fmt.Fprintln(output, "  -local-slow-after          env ALLROUTER_LOCAL_SLOW_AFTER        default 0s")
		fmt.Fprintln(output, "  -burst-on-error            env ALLROUTER_BURST_ON_ERROR          default true")
		fmt.Fprintln(output, "  -burst-fallback-model      env ALLROUTER_BURST_FALLBACK_MODEL    default \"\"")
		fmt.Fprintln(output, "  -preset                    env ALLROUTER_PRESET                  default \"\"")
		fmt.Fprintln(output, "  -backup-model              env ALLROUTER_BACKUP_MODELS           repeatable; BackupRouter defaults to Kimi K3, GLM 5.2")
		fmt.Fprintln(output, "  -token                     env ALLROUTER_TOKEN                   default \"\"")
		fmt.Fprintln(output, "  -alias                     env ALLROUTER_ALIASES                 default \"\"")
		fmt.Fprintln(output, "  -savings-reference         env ALLROUTER_SAVINGS_REFERENCE       default \"\"")
		fmt.Fprintln(output, "  -state-file                env ALLROUTER_STATE_FILE              default $XDG_STATE_HOME/allrouter/state.json or ~/.allrouter/state.json")
		fmt.Fprintln(output, "  -config-file               env ALLROUTER_CONFIG_FILE             default $XDG_CONFIG_HOME/allrouter/config.json or ~/.config/allrouter/config.json")
		fmt.Fprintln(output, "  -cloud                     env ALLROUTER_CLOUD                   default auto")
		fmt.Fprintln(output, "  -max-cloud-spend           env ALLROUTER_MAX_CLOUD_SPEND         default 0")
		fmt.Fprintln(output, "  -sse-batch-window          env ALLROUTER_SSE_BATCH_WINDOW        default 0s")
		fmt.Fprintln(output, "  -sse-batch-max-bytes       env ALLROUTER_SSE_BATCH_MAX_BYTES     default 4096")
		fmt.Fprintln(output, "  -no-autodetect             env none                           default false")
		fmt.Fprintln(output, "  -version                   env none                           default false")
	}
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.Preset == PresetBackupRouter && !cfg.backupModelsExplicit {
		models, found, err := LoadRuntimeConfig(cfg.ConfigFile)
		if err != nil {
			return Config{}, fmt.Errorf("-config-file: %w", err)
		}
		if found {
			cfg.BackupModels = models
		}
	}
	applyPreset(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultsFromEnv(lookupEnv func(string) (string, bool)) (Config, error) {
	cfg := Config{
		Listen:              ":8383",
		TRBaseURL:           trustedrouter.DefaultAPIBaseURL,
		TRCatalogURL:        DefaultTRCatalogURL,
		LocalMaxConcurrency: 4,
		LocalQueueWait:      0,
		BurstOnError:        true,
		Aliases:             map[string]string{},
		StateFile:           defaultStateFile(lookupEnv),
		ConfigFile:          defaultConfigFile(lookupEnv),
		Cloud:               CloudAuto,
		SSEBatchMaxBytes:    4096,
	}

	if value, ok := lookupEnv(envListen); ok {
		cfg.Listen = value
	}
	if value, ok := lookupEnv(envLocalURL); ok {
		cfg.LocalURL = value
	}
	if value, ok := lookupEnv(envTRAPIKey); ok {
		cfg.TRAPIKey = value
	}
	if value, ok := lookupEnv(envTRBaseURL); ok {
		cfg.TRBaseURL = value
	}
	if value, ok := lookupEnv(envTRCatalogURL); ok {
		cfg.TRCatalogURL = value
	}
	if value, ok := lookupEnv(envLocalMaxConcurrency); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envLocalMaxConcurrency, err)
		}
		cfg.LocalMaxConcurrency = parsed
	}
	if value, ok := lookupEnv(envLocalQueueWait); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envLocalQueueWait, err)
		}
		cfg.LocalQueueWait = parsed
	}
	if value, ok := lookupEnv(envLocalSlowAfter); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envLocalSlowAfter, err)
		}
		cfg.LocalSlowAfter = parsed
	}
	if value, ok := lookupEnv(envSSEBatchWindow); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envSSEBatchWindow, err)
		}
		cfg.SSEBatchWindow = parsed
	}
	if value, ok := lookupEnv(envSSEBatchMaxBytes); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envSSEBatchMaxBytes, err)
		}
		cfg.SSEBatchMaxBytes = parsed
	}
	if value, ok := lookupEnv(envBurstOnError); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envBurstOnError, err)
		}
		cfg.BurstOnError = parsed
	}
	if value, ok := lookupEnv(envBurstFallbackModel); ok {
		cfg.BurstFallbackModel = value
	}
	if value, ok := lookupEnv(envPreset); ok {
		if err := parsePreset(value, &cfg.Preset); err != nil {
			return Config{}, fmt.Errorf("%s: %w", envPreset, err)
		}
	}
	if value, ok := lookupEnv(envBackupModels); ok {
		if strings.TrimSpace(value) != "" {
			models, err := parseModelList(value)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", envBackupModels, err)
			}
			cfg.BackupModels = models
			cfg.backupModelsExplicit = true
		}
	}
	if value, ok := lookupEnv(envToken); ok {
		cfg.Token = value
	}
	if value, ok := lookupEnv(envAliases); ok {
		if err := parseAliasList(value, cfg.Aliases); err != nil {
			return Config{}, fmt.Errorf("%s: %w", envAliases, err)
		}
	}
	if value, ok := lookupEnv(envSavingsReference); ok {
		cfg.SavingsReference = value
	}
	if value, ok := lookupEnv(envStateFile); ok {
		cfg.StateFile = value
	}
	if value, ok := lookupEnv(envConfigFile); ok {
		cfg.ConfigFile = value
	}
	if value, ok := lookupEnv(envCloud); ok {
		if err := parseCloudMode(value, &cfg.Cloud); err != nil {
			return Config{}, fmt.Errorf("%s: %w", envCloud, err)
		}
	}
	if value, ok := lookupEnv(envMaxCloudSpend); ok {
		parsed, err := parseUSDMicro(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", envMaxCloudSpend, err)
		}
		cfg.MaxCloudSpendMicro = parsed
	}

	return cfg, nil
}

func validate(cfg Config) error {
	if strings.TrimSpace(cfg.Listen) == "" {
		return errors.New("-listen must not be empty")
	}
	if cfg.LocalMaxConcurrency < 1 {
		return errors.New("-local-max-concurrency must be at least 1")
	}
	if cfg.LocalQueueWait < 0 {
		return errors.New("-local-queue-wait must not be negative")
	}
	if cfg.LocalSlowAfter < 0 {
		return errors.New("-local-slow-after must not be negative")
	}
	if err := validateCloudMode(cfg.Cloud); err != nil {
		return err
	}
	if cfg.MaxCloudSpendMicro < 0 {
		return errors.New("-max-cloud-spend must not be negative")
	}
	if cfg.SSEBatchWindow < 0 {
		return errors.New("-sse-batch-window must not be negative")
	}
	if err := validatePreset(cfg.Preset); err != nil {
		return err
	}
	if err := validateBackupModels(cfg.BackupModels); err != nil {
		return err
	}
	if cfg.Preset == PresetBackupRouter && cfg.Cloud != CloudAuto {
		return errors.New("-preset=backuprouter requires -cloud=auto")
	}
	return nil
}

// ValidateRuntime verifies the final startup configuration after local
// autodetection has had a chance to populate LocalURL.
func ValidateRuntime(cfg Config) error {
	if cfg.Preset == PresetBackupRouter && !cfg.HasTrustedRouter() {
		return errors.New("-preset=backuprouter requires TRUSTEDROUTER_API_KEY or -tr-api-key")
	}
	if strings.TrimSpace(cfg.LocalURL) != "" || strings.TrimSpace(cfg.TRAPIKey) != "" {
		return nil
	}
	if cfg.NoAutodetect {
		return errors.New("no upstream configured: pass -local-url http://127.0.0.1:11434, set TRUSTEDROUTER_API_KEY, or remove -no-autodetect to probe local servers")
	}
	return errors.New("no local OpenAI-compatible server found and TRUSTEDROUTER_API_KEY is unset: start Ollama, LM Studio, llama.cpp, or vLLM; pass -local-url; or set TRUSTEDROUTER_API_KEY for cloud passthrough")
}

func defaultStateFile(lookupEnv func(string) (string, bool)) string {
	if stateHome, ok := lookupEnv("XDG_STATE_HOME"); ok && strings.TrimSpace(stateHome) != "" {
		return filepath.Join(stateHome, "allrouter", "state.json")
	}
	if home, ok := lookupEnv("HOME"); ok && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".allrouter", "state.json")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".allrouter", "state.json")
	}
	return ""
}

func defaultConfigFile(lookupEnv func(string) (string, bool)) string {
	if configHome, ok := lookupEnv("XDG_CONFIG_HOME"); ok && strings.TrimSpace(configHome) != "" {
		return filepath.Join(configHome, "allrouter", "config.json")
	}
	if home, ok := lookupEnv("HOME"); ok && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config", "allrouter", "config.json")
	}
	return ""
}

func applyPreset(cfg *Config) {
	if cfg.Preset == PresetBackupRouter && len(cfg.BackupModels) == 0 {
		cfg.BackupModels = append([]string(nil), defaultBackupModels...)
	}
}

type presetValue struct {
	value *Preset
}

func (v presetValue) String() string {
	if v.value == nil {
		return ""
	}
	return string(*v.value)
}

func (v presetValue) Set(value string) error {
	return parsePreset(value, v.value)
}

func parsePreset(value string, dst *Preset) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "none":
		*dst = PresetNone
	case "backuprouter", "backup-router", "backup":
		*dst = PresetBackupRouter
	default:
		return errors.New("-preset must be backuprouter")
	}
	return nil
}

func validatePreset(preset Preset) error {
	switch preset {
	case PresetNone, PresetBackupRouter:
		return nil
	default:
		return errors.New("-preset must be backuprouter")
	}
}

type modelListValue struct {
	values   *[]string
	explicit *bool
	seen     bool
}

func (v *modelListValue) String() string {
	if v == nil || v.values == nil {
		return ""
	}
	return strings.Join(*v.values, ",")
}

func (v *modelListValue) Set(value string) error {
	models, err := parseModelList(value)
	if err != nil {
		return err
	}
	if !v.seen {
		*v.values = nil
		v.seen = true
	}
	if v.explicit != nil {
		*v.explicit = true
	}
	*v.values = append(*v.values, models...)
	return validateBackupModels(*v.values)
}

func parseModelList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" {
			return nil, errors.New("backup model must not be empty")
		}
		models = append(models, model)
	}
	if err := validateBackupModels(models); err != nil {
		return nil, err
	}
	return models, nil
}

func validateBackupModels(models []string) error {
	if len(models) > MaxBackupModels {
		return fmt.Errorf("at most %d backup models are allowed", MaxBackupModels)
	}
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			return errors.New("backup model must not be empty")
		}
		if strings.HasPrefix(strings.ToLower(model), "local/") {
			return fmt.Errorf("backup model %q must be a cloud model", model)
		}
		if _, exists := seen[model]; exists {
			return fmt.Errorf("duplicate backup model %q", model)
		}
		seen[model] = struct{}{}
	}
	return nil
}

// NormalizeBackupModels trims and validates an ordered runtime fallback list.
func NormalizeBackupModels(models []string) ([]string, error) {
	normalized := make([]string, len(models))
	for index, model := range models {
		normalized[index] = strings.TrimSpace(model)
	}
	if err := validateBackupModels(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

type aliasMapValue struct {
	values map[string]string
}

func (v aliasMapValue) String() string {
	if len(v.values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(v.values))
	for key := range v.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+v.values[key])
	}
	return strings.Join(parts, ",")
}

func (v aliasMapValue) Set(value string) error {
	from, to, err := parseAliasPair(value)
	if err != nil {
		return err
	}
	if _, ok := v.values[from]; ok {
		return fmt.Errorf("duplicate alias %q", from)
	}
	v.values[from] = to
	return nil
}

func parseAliasList(value string, aliases map[string]string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parser := aliasMapValue{values: aliases}
	for _, part := range strings.Split(value, ",") {
		if err := parser.Set(part); err != nil {
			return err
		}
	}
	return nil
}

func parseAliasPair(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if strings.Count(value, "=") != 1 {
		return "", "", fmt.Errorf("alias %q must have from=to shape", value)
	}
	from, to, _ := strings.Cut(value, "=")
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return "", "", fmt.Errorf("alias %q must have non-empty from and to", value)
	}
	return from, to, nil
}

type cloudModeValue struct {
	value *CloudMode
}

func (v cloudModeValue) String() string {
	if v.value == nil {
		return ""
	}
	return string(*v.value)
}

func (v cloudModeValue) Set(value string) error {
	return parseCloudMode(value, v.value)
}

func parseCloudMode(value string, dst *CloudMode) error {
	mode := CloudMode(strings.ToLower(strings.TrimSpace(value)))
	if err := validateCloudMode(mode); err != nil {
		return err
	}
	*dst = mode
	return nil
}

func validateCloudMode(mode CloudMode) error {
	switch mode {
	case CloudAuto, CloudExplicit, CloudOff:
		return nil
	default:
		return fmt.Errorf("-cloud must be one of auto, explicit, off")
	}
}

type usdMicroValue struct {
	value *int64
}

func (v usdMicroValue) String() string {
	if v.value == nil {
		return ""
	}
	return formatUSDMicro(*v.value)
}

func (v usdMicroValue) Set(value string) error {
	parsed, err := parseUSDMicro(value)
	if err != nil {
		return err
	}
	*v.value = parsed
	return nil
}

func parseUSDMicro(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty USD value")
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, errors.New("USD value must not be negative")
	}
	micro := parsed * 1_000_000
	if micro > float64(math.MaxInt64) {
		return 0, errors.New("USD value is too large")
	}
	return int64(math.Round(micro)), nil
}

func formatUSDMicro(value int64) string {
	return strconv.FormatFloat(float64(value)/1_000_000, 'f', 6, 64)
}
