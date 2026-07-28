package proxy

import (
	"fmt"
	"net/http"
	"strings"
)

type savingsMetrics struct {
	savedUSD              string
	cloudSpendUSD         string
	localPromptTokens     int64
	localCompletionTokens int64
	usageUnknownTotal     int64
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", "invalid_request_error")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(s.metricsText()))
}

func (s *Server) metricsText() string {
	var b strings.Builder

	writePromHelpType(&b, "hybrid_requests_total", "Total routed requests accepted by HybridRouter.", "counter")
	writePromInt(&b, "hybrid_requests_total", "", s.stats.requestsTotal.Value())

	writePromHelpType(&b, "hybrid_in_flight_local", "Current requests occupying local upstream slots.", "gauge")
	writePromInt(&b, "hybrid_in_flight_local", "", s.stats.inFlightLocal.Value())

	writePromHelpType(&b, "hybrid_route_total", "Total completed upstream responses by route.", "counter")
	writePromInt(&b, "hybrid_route_total", `{route="local"}`, s.stats.routes.local.Value())
	writePromInt(&b, "hybrid_route_total", `{route="trustedrouter"}`, s.stats.routes.tr.Value())

	writePromHelpType(&b, "hybrid_bursts_total", "Total burst outcomes by reason.", "counter")
	writePromInt(&b, "hybrid_bursts_total", `{reason="full"}`, s.stats.burstsFull.Value())
	writePromInt(&b, "hybrid_bursts_total", `{reason="error"}`, s.stats.burstsError.Value())
	writePromInt(&b, "hybrid_bursts_total", `{reason="skipped_unmapped"}`, s.stats.burstsSkippedUnmapped.Value())

	savings := collectSavingsMetrics(s.savings)
	writePromHelpType(&b, "hybrid_saved_usd_total", "Estimated cumulative USD saved by local routing.", "counter")
	writePromFloat(&b, "hybrid_saved_usd_total", "", savings.savedUSD)

	writePromHelpType(&b, "hybrid_cloud_spend_usd_total", "Estimated cumulative USD spent on cloud routing.", "counter")
	writePromFloat(&b, "hybrid_cloud_spend_usd_total", "", savings.cloudSpendUSD)

	writePromHelpType(&b, "hybrid_local_tokens_total", "Total local tokens by usage kind.", "counter")
	writePromInt(&b, "hybrid_local_tokens_total", `{kind="prompt"}`, savings.localPromptTokens)
	writePromInt(&b, "hybrid_local_tokens_total", `{kind="completion"}`, savings.localCompletionTokens)

	writePromHelpType(&b, "hybrid_usage_unknown_total", "Total responses where token usage was unavailable.", "counter")
	writePromInt(&b, "hybrid_usage_unknown_total", "", savings.usageUnknownTotal)

	writePromHelpType(&b, "hybrid_cloud_blocked_total", "Total cloud sends blocked by reason.", "counter")
	writePromInt(&b, "hybrid_cloud_blocked_total", `{reason="budget"}`, s.stats.cloudBlockedBudget.Value())
	writePromInt(&b, "hybrid_cloud_blocked_total", `{reason="mode"}`, s.stats.cloudBlockedMode.Value())

	return b.String()
}

func collectSavingsMetrics(m *savingsMeter) savingsMetrics {
	if m == nil {
		return savingsMetrics{savedUSD: "0.000000", cloudSpendUSD: "0.000000"}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return savingsMetrics{
		savedUSD:              formatUSDMicro(m.state.SavedUSDMicro),
		cloudSpendUSD:         formatUSDMicro(m.state.CloudSpendUSDMicro),
		localPromptTokens:     m.state.LocalPromptTokens,
		localCompletionTokens: m.state.LocalCompletionTokens,
		usageUnknownTotal:     m.usageUnknownTotal,
	}
}

func writePromHelpType(b *strings.Builder, name, help, typ string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, typ)
}

func writePromInt(b *strings.Builder, name, labels string, value int64) {
	fmt.Fprintf(b, "%s%s %d\n", name, labels, value)
}

func writePromFloat(b *strings.Builder, name, labels, value string) {
	fmt.Fprintf(b, "%s%s %s\n", name, labels, value)
}
