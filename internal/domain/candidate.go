package domain

type CandidateKind string

const (
	CandidateTorrent CandidateKind = "torrent"
	CandidateDirect  CandidateKind = "direct"
	CandidateUsenet  CandidateKind = "usenet"
)

type MediaRef struct {
	Type string
	ID   string
}

type TorrentInfo struct {
	InfoHash  string
	FileIndex *int
}

type UsenetInfo struct {
	NZBURL      string
	Hash        string
	Indexer     string
	FileIndex   *int
	EasynewsURL string
}

type HTTPStream struct {
	URL     string
	Headers map[string]string
}

type Candidate struct {
	ID         string
	SourceID   string
	Kind       CandidateKind
	Media      MediaRef
	Title      string
	Filename   string
	SizeBytes  int64
	Resolution string
	Codec      string
	Languages  []string
	Cached     *bool
	Seeders    *int
	Torrent    *TorrentInfo
	Usenet     *UsenetInfo
	HTTP       *HTTPStream
	Score      float64
}
