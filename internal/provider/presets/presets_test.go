package presets

import "testing"

func TestResolveUsesPresetDefault(t *testing.T) {
	got, err := Resolve("torrentio", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "https://torrentio.strem.fun" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestResolveAllowsConfiguredOverride(t *testing.T) {
	const configured = "https://comet.example/encoded-config/manifest.json"
	got, err := Resolve("comet", configured)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != configured {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestResolveRejectsUnknownPreset(t *testing.T) {
	if _, err := Resolve("unknown", ""); err == nil {
		t.Fatal("expected error")
	}
}
