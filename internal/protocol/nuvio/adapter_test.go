package nuvio

import (
	"testing"

	"streamweave/internal/engine"
)

func TestManifestClientModeAdvertisesP2P(t *testing.T) {
	hints := (Adapter{}).Manifest(engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionClient,
	})
	if !hints.P2P || hints.P2PNotSupported {
		t.Fatalf("hints = %#v", hints)
	}
	if len(hints.IDPrefixes) != 2 || hints.IDPrefixes[0] != "tt" || hints.IDPrefixes[1] != "tmdb" {
		t.Fatalf("id prefixes = %#v", hints.IDPrefixes)
	}
}

func TestManifestServerModeDisablesP2P(t *testing.T) {
	hints := (Adapter{}).Manifest(engine.InstallationProfile{
		ClientMode:     "nuvio",
		ResolutionMode: engine.ResolutionServer,
	})
	if hints.P2P || !hints.P2PNotSupported {
		t.Fatalf("hints = %#v", hints)
	}
}

func TestUsesNativeDebridOnlyInNuvioClientMode(t *testing.T) {
	adapter := Adapter{}
	if !adapter.UsesNativeDebrid(engine.InstallationProfile{ClientMode: "nuvio", ResolutionMode: engine.ResolutionClient}) {
		t.Fatal("expected Nuvio client mode to use native debrid")
	}
	if adapter.UsesNativeDebrid(engine.InstallationProfile{ClientMode: "nuvio", ResolutionMode: engine.ResolutionServer}) {
		t.Fatal("server mode must not use Nuvio native debrid")
	}
}
