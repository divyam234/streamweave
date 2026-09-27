package presets

import (
	"fmt"
	"strings"
)

type Preset struct {
	ID       string
	Name     string
	Endpoint string
}

var catalog = map[string]Preset{
	"torrentio": {
		ID:       "torrentio",
		Name:     "Torrentio",
		Endpoint: "https://torrentio.strem.fun",
	},
	"comet": {
		ID:       "comet",
		Name:     "Comet",
		Endpoint: "https://comet.elfhosted.com",
	},
	"mediafusion": {
		ID:       "mediafusion",
		Name:     "MediaFusion",
		Endpoint: "https://mediafusion.elfhosted.com",
	},
}

func Lookup(id string) (Preset, bool) {
	preset, ok := catalog[strings.ToLower(strings.TrimSpace(id))]
	return preset, ok
}

func Resolve(id, endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if strings.TrimSpace(id) == "" {
		return endpoint, nil
	}

	preset, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("unknown remote addon preset %q", id)
	}
	if endpoint != "" {
		return endpoint, nil
	}
	return preset.Endpoint, nil
}
