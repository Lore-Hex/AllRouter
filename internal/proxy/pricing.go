package proxy

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/Lore-Hex/AllRouter/internal/policy"
)

func (s *Server) localSavingsPrice(ctx context.Context, decision policy.Decision) priceQuote {
	for _, model := range s.localPricingCandidates(decision) {
		if quote, ok := s.catalogPrice(ctx, model); ok {
			return quote
		}
	}
	return priceQuote{}
}

func (s *Server) localPricingCandidates(decision policy.Decision) []string {
	var candidates []string
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		for _, existing := range candidates {
			if existing == model {
				return
			}
		}
		candidates = append(candidates, model)
	}
	if decision.AliasKey != "" {
		add(decision.AliasKey)
	}
	if trustedRouterModelCandidate(decision.View.Model) {
		add(decision.View.Model)
	}
	if s.cfg.SavingsReference != "" {
		add(s.cfg.SavingsReference)
	}
	return candidates
}

func trustedRouterModelCandidate(model string) bool {
	model = strings.TrimSpace(model)
	return model != "" && !strings.HasPrefix(model, "local/")
}

func (s *Server) cloudSavingsPrice(ctx context.Context, model string) priceQuote {
	quote, _ := s.catalogPrice(ctx, model)
	return quote
}

func (s *Server) catalogPrice(ctx context.Context, model string) (priceQuote, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return priceQuote{}, false
	}
	// Pricing runs after the response has been served, when the request context
	// is often already canceled (the client is gone). Detach from cancellation
	// so the catalog fetch still succeeds, with its own timeout so a record can
	// never hang the handler.
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), catalogTimeout)
	defer cancel()
	models, err := s.cachedTrustedRouterModels(fetchCtx)
	if err != nil {
		return priceQuote{}, false
	}
	for _, entry := range models {
		id, _ := entry["id"].(string)
		if id != model {
			continue
		}
		quote, ok := priceFromModel(entry)
		if !ok {
			return priceQuote{}, false
		}
		quote.Reference = id
		return quote, true
	}
	return priceQuote{}, false
}

func priceFromModel(entry map[string]any) (priceQuote, bool) {
	pricing, _ := entry["pricing"].(map[string]any)
	prompt, promptOK := priceMicroPerToken(pricing, entry, "prompt", "prompt_max", "prompt_usd_per_mtok", "input_usd_per_mtok", "input_price_per_mtok")
	completion, completionOK := priceMicroPerToken(pricing, entry, "completion", "completion_max", "completion_usd_per_mtok", "output_usd_per_mtok", "output_price_per_mtok")
	if !promptOK || !completionOK {
		return priceQuote{}, false
	}
	return priceQuote{
		PromptMicroPerToken:     prompt,
		CompletionMicroPerToken: completion,
		Priced:                  true,
	}, true
}

func priceMicroPerToken(pricing map[string]any, entry map[string]any, tokenPriceKey, maxTokenPriceKey string, perMTokKeys ...string) (float64, bool) {
	for _, key := range perMTokKeys {
		if value, ok := numericField(pricing, key); ok {
			return value, true
		}
		if value, ok := numericField(entry, key); ok {
			return value, true
		}
	}
	if value, ok := numericField(pricing, tokenPriceKey); ok {
		return value * 1_000_000, true
	}
	if value, ok := numericField(pricing, maxTokenPriceKey); ok {
		return value * 1_000_000, true
	}
	return 0, false
}

// usableTokenPrice rejects a per-token price that cannot be spent against.
//
// Prices arrive from the model catalog over HTTP, so they are untrusted input.
// strconv.ParseFloat accepts "NaN", "Inf" and hex-float forms, and Go's
// float64->int64 conversion SATURATES rather than erroring, so a hostile or
// merely broken catalog value reaches the spend accumulator as either
// math.MaxInt64 or a negative number.
//
// Both break -max-cloud-spend, which is the one hard egress-cost control this
// proxy ships: a negative price makes the day's accumulated spend go DOWN on
// every request, so the cap never trips, and the poisoned total is persisted
// to disk and survives a restart. Reject instead, and fall back to "unknown
// price" — the caller already handles that.
// maxTokenPriceUSD is a sanity ceiling, not a business rule. A per-token price
// above a million dollars is a broken catalog, not an expensive model, and
// admitting one lets the int64 microdollar accumulator saturate: ParseFloat
// accepts hex-float forms like "0x1p1000" (1.07e+301), which multiplied by a
// token count and converted to int64 pins spend at math.MaxInt64 on a single
// request. That trips the cap permanently rather than disabling it — fail
// closed rather than open, but still wrong, and still driven by a value the
// catalog chose.
const maxTokenPriceUSD = 1e6

func usableTokenPrice(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	if value > maxTokenPriceUSD {
		return 0, false
	}
	return value, true
}

func numericField(fields map[string]any, key string) (float64, bool) {
	if fields == nil {
		return 0, false
	}
	switch value := fields[key].(type) {
	case float64:
		return usableTokenPrice(value)
	case int:
		return usableTokenPrice(float64(value))
	case int64:
		return usableTokenPrice(float64(value))
	case json.Number:
		parsed, err := strconv.ParseFloat(string(value), 64)
		if err != nil {
			return 0, false
		}
		return usableTokenPrice(parsed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, false
		}
		return usableTokenPrice(parsed)
	default:
		return 0, false
	}
}
