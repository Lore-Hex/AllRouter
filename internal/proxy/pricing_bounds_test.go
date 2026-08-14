package proxy

import (
	"encoding/json"
	"math"
	"testing"
)

// Catalog prices are untrusted input: they arrive over HTTP and land in an
// int64 spend accumulator that backs -max-cloud-spend, the one hard egress-cost
// control this proxy ships.
//
// Two Go behaviours make a bad value dangerous rather than merely wrong.
// strconv.ParseFloat accepts "NaN", "Inf" and hex-float forms, and float64 ->
// int64 conversion SATURATES instead of erroring. A negative price makes the
// day's accumulated spend go DOWN on every request, so the cap never trips —
// and the poisoned total is written to disk and survives a restart.
//
// The law: a price is usable only if it is finite and non-negative.

func TestNumericFieldRejectsUnusablePrices(t *testing.T) {
	for _, raw := range []any{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		-0.000001,
		-1.0,
		"NaN",
		"Inf",
		"-Inf",
		"infinity",
		"-5",
		"0x1p1000",
		json.Number("NaN"),
		json.Number("-1"),
	} {
		if _, ok := numericField(map[string]any{"p": raw}, "p"); ok {
			t.Fatalf("price %v (%T) was accepted; it must fall back to unknown", raw, raw)
		}
	}
}

func TestNumericFieldStillAcceptsRealPrices(t *testing.T) {
	// The guard must not disturb the values it was not aimed at.
	for raw, want := range map[any]float64{
		0.0:                0,
		0.0000025:          0.0000025,
		3.5:                3.5,
		int(7):             7,
		int64(11):          11,
		"0.000002":         0.000002,
		" 1.5 ":            1.5,
		json.Number("2.5"): 2.5,
	} {
		got, ok := numericField(map[string]any{"p": raw}, "p")
		if !ok || got != want {
			t.Fatalf("price %v (%T): got (%v, %v), want (%v, true)", raw, raw, got, ok, want)
		}
	}
}

func TestAnUnusablePriceCannotDriveSpendNegative(t *testing.T) {
	// The consequence that matters: a rejected price yields "unknown", which the
	// caller treats as no-cost rather than as a negative cost. Nothing the
	// catalog can say may reduce accumulated spend.
	for _, raw := range []any{-1.0, math.Inf(-1), "-99999"} {
		value, ok := numericField(map[string]any{"p": raw}, "p")
		if ok && value < 0 {
			t.Fatalf("price %v produced a negative usable value %v", raw, value)
		}
	}
}

func TestUsableTokenPriceIsTotal(t *testing.T) {
	for _, v := range []float64{
		0, 1, 1e-9, 1e9, math.MaxFloat64,
		math.NaN(), math.Inf(1), math.Inf(-1), -1, -math.MaxFloat64,
	} {
		got, ok := usableTokenPrice(v)
		if ok && (math.IsNaN(got) || math.IsInf(got, 0) || got < 0) {
			t.Fatalf("usableTokenPrice(%v) returned (%v, true)", v, got)
		}
	}
}
