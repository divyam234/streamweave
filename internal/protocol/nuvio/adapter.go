package nuvio

import "media-engine/internal/engine"

type ManifestHints struct {
	IDPrefixes      []string
	P2P             bool
	P2PNotSupported bool
	Configurable    bool
}

type Adapter struct{}

func (Adapter) Manifest(profile engine.InstallationProfile) ManifestHints {
	hints := ManifestHints{
		IDPrefixes:   []string{"tt", "tmdb"},
		Configurable: false,
	}
	switch profile.ResolutionMode {
	case engine.ResolutionServer:
		hints.P2P = false
		hints.P2PNotSupported = true
	case engine.ResolutionClient, engine.ResolutionHybrid:
		hints.P2P = true
		hints.P2PNotSupported = false
	default:
		hints.P2P = true
		hints.P2PNotSupported = false
	}
	return hints
}

func (Adapter) UsesNativeDebrid(profile engine.InstallationProfile) bool {
	return profile.ClientMode == "nuvio" && profile.ResolutionMode == engine.ResolutionClient
}
