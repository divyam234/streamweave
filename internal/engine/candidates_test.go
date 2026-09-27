package engine

import (
	"testing"

	"streamweave/internal/domain"
)

func TestPrepareCandidatesEnrichesDeduplicatesAndRanks(t *testing.T) {
	index := 2
	seeders := 50
	candidates := []domain.Candidate{
		{ID: "slow", SourceID: "z-provider", Kind: domain.CandidateTorrent, Title: "Movie.1080p.x264.2.0 GB", Torrent: &domain.TorrentInfo{InfoHash: "ABCDEF", FileIndex: &index}},
		{ID: "duplicate", SourceID: "a-provider", Kind: domain.CandidateTorrent, Title: "Movie 1080p HEVC", Seeders: &seeders, Torrent: &domain.TorrentInfo{InfoHash: "abcdef", FileIndex: &index}},
		{ID: "best", SourceID: "b-provider", Kind: domain.CandidateDirect, Title: "Movie.2160p.AV1", HTTP: &domain.HTTPStream{URL: "https://cdn.example/movie.mkv"}},
	}
	got := prepareCandidates(candidates)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "best" {
		t.Fatalf("first = %q, want best", got[0].ID)
	}
	if got[0].Resolution != "2160p" || got[0].Codec != "av1" {
		t.Fatalf("unexpected enrichment: %#v", got[0])
	}
	if got[1].Seeders == nil || *got[1].Seeders != 50 {
		t.Fatalf("dedupe did not merge seeders: %#v", got[1])
	}
	if got[1].SizeBytes == 0 {
		t.Fatal("expected parsed size")
	}
}

func TestPrepareCandidatesTieBreakIsDeterministic(t *testing.T) {
	input := []domain.Candidate{
		{ID: "b", SourceID: "same", Kind: domain.CandidateTorrent, Torrent: &domain.TorrentInfo{InfoHash: "b"}},
		{ID: "a", SourceID: "same", Kind: domain.CandidateTorrent, Torrent: &domain.TorrentInfo{InfoHash: "a"}},
	}
	got := prepareCandidates(input)
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("order = %q, %q", got[0].ID, got[1].ID)
	}
}

func TestEnglishPreferredWithoutHidingOtherLanguages(t *testing.T) {
	got := prepareCandidates([]domain.Candidate{
		{ID: "foreign", SourceID: "source", Title: "Movie.French.2160p", Torrent: &domain.TorrentInfo{InfoHash: "a"}},
		{ID: "unknown", SourceID: "source", Title: "Movie.1080p", Torrent: &domain.TorrentInfo{InfoHash: "b"}},
		{ID: "english", SourceID: "source", Title: "Movie.English.720p", Torrent: &domain.TorrentInfo{InfoHash: "c"}},
	})
	if len(got) != 3 || got[0].ID != "english" || got[1].ID != "unknown" || got[2].ID != "foreign" {
		t.Fatalf("unexpected order: %#v", got)
	}
}
