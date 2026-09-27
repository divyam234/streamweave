package stremio

type Manifest struct {
	ID            string                 `json:"id"`
	Version       string                 `json:"version"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Resources     []string               `json:"resources"`
	Types         []string               `json:"types"`
	IDPrefixes    []string               `json:"idPrefixes,omitempty"`
	Catalogs      []Catalog              `json:"catalogs"`
	BehaviorHints *ManifestBehaviorHints `json:"behaviorHints,omitempty"`
}

type ManifestBehaviorHints struct {
	Configurable          bool `json:"configurable"`
	ConfigurationRequired bool `json:"configurationRequired,omitempty"`
	Adult                 bool `json:"adult,omitempty"`
	P2P                   bool `json:"p2p,omitempty"`
	P2PNotSupported       bool `json:"p2pNotSupported,omitempty"`
}

type Catalog struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type ProxyHeaders struct {
	Request map[string]string `json:"request,omitempty"`
}

type BehaviorHints struct {
	BingeGroup   string        `json:"bingeGroup,omitempty"`
	NotWebReady  bool          `json:"notWebReady,omitempty"`
	VideoHash    string        `json:"videoHash,omitempty"`
	VideoSize    int64         `json:"videoSize,omitempty"`
	Filename     string        `json:"filename,omitempty"`
	ProxyHeaders *ProxyHeaders `json:"proxyHeaders,omitempty"`
}

type Stream struct {
	Name          string         `json:"name,omitempty"`
	Title         string         `json:"title,omitempty"`
	URL           string         `json:"url,omitempty"`
	InfoHash      string         `json:"infoHash,omitempty"`
	FileIdx       *int           `json:"fileIdx,omitempty"`
	BehaviorHints *BehaviorHints `json:"behaviorHints,omitempty"`
}

type StreamResponse struct {
	Streams []Stream `json:"streams"`
}
