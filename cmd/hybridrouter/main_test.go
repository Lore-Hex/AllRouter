package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Lore-Hex/HybridRouter/internal/config"
	"github.com/Lore-Hex/HybridRouter/internal/proxy"
)

func TestPrintBootBannerGolden(t *testing.T) {
	oldVersion := version
	version = "v1.2.3"
	t.Cleanup(func() { version = oldVersion })

	var out bytes.Buffer
	printBootBanner(&out, config.Config{
		TRAPIKey:           "tr-key",
		TRBaseURL:          "https://api.quillrouter.com/v1",
		Cloud:              config.CloudExplicit,
		MaxCloudSpendMicro: 1_250_000,
	}, localBannerInfo{
		URL:             "http://127.0.0.1:11434/v1",
		Flavor:          "Ollama",
		ModelCount:      3,
		ModelCountKnown: true,
		Autodetected:    true,
	}, proxy.SavingsTotals{
		SavedUSDMicro:      12_345_678,
		CloudSpendUSDMicro: 123_456,
		TopReference:       "gpt-4o",
		HasHistory:         true,
	})

	got := out.String()
	for _, want := range []string{
		"HybridRouter v1.2.3",
		"local: detected Ollama at http://127.0.0.1:11434/v1 (3 models)",
		"cloud: api.quillrouter.com",
		"mode: cloud=explicit, max-cloud-spend=$1.250000/day",
		"savings: saved $12.345678 (ref: gpt-4o), cloud spend $0.123456",
		"Point your tools at http://localhost:8383/v1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("banner missing %q:\n%s", want, got)
		}
	}
	if lines := strings.Count(strings.TrimSpace(got), "\n") + 1; lines > 10 {
		t.Fatalf("banner has %d lines, want <= 10:\n%s", lines, got)
	}
}

func TestClientBaseURLUsesConfiguredListener(t *testing.T) {
	for _, test := range []struct {
		listen string
		want   string
	}{
		{listen: ":8383", want: "http://localhost:8383/v1"},
		{listen: "0.0.0.0:9000", want: "http://localhost:9000/v1"},
		{listen: "127.0.0.1:18383", want: "http://127.0.0.1:18383/v1"},
		{listen: "[::1]:8383", want: "http://[::1]:8383/v1"},
	} {
		if got := clientBaseURL(test.listen); got != test.want {
			t.Fatalf("clientBaseURL(%q) = %q, want %q", test.listen, got, test.want)
		}
	}
}

func TestConfiguredLocalInfoDoesNotRequireProbe(t *testing.T) {
	info := configuredLocalInfo("127.0.0.1:11434")
	if info.URL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("URL = %q", info.URL)
	}
	if info.Flavor != "Ollama" {
		t.Fatalf("Flavor = %q", info.Flavor)
	}
	if info.ModelCountKnown {
		t.Fatal("ModelCountKnown = true, want false")
	}
}

func TestModeDisplayBackupRouter(t *testing.T) {
	got := modeDisplay(config.Config{
		Preset:       config.PresetBackupRouter,
		BackupModels: []string{"moonshotai/kimi-k3", "z-ai/glm-5.2"},
		Cloud:        config.CloudAuto,
	})
	want := "preset=backuprouter, backups=moonshotai/kimi-k3,z-ai/glm-5.2"
	if got != want {
		t.Fatalf("modeDisplay() = %q, want %q", got, want)
	}
}
