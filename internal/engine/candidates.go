package engine

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"streamweave/internal/domain"
)

var (
	resolutionPattern = regexp.MustCompile(`(?i)(?:^|[ ._-]|\[|\()(2160p|4k|1080p|720p|480p)(?:$|[ ._-]|\]|\))`)
	codecPattern      = regexp.MustCompile(`(?i)(?:^|[ ._-]|\[|\()(av1|hevc|h[ ._-]?265|x265|h[ ._-]?264|x264)(?:$|[ ._-]|\]|\))`)
	sizePattern       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(tb|gb|mb)\b`)
	englishPattern    = regexp.MustCompile(`(?i)(?:^|[^a-z])(eng|english)(?:$|[^a-z])`)
	foreignPattern    = regexp.MustCompile(`(?i)(?:^|[^a-z])(hindi|tamil|telugu|french|german|spanish|italian|japanese|korean|russian|arabic|dubbed)(?:$|[^a-z])`)
)

func prepareCandidates(candidates []domain.Candidate) []domain.Candidate {
	if len(candidates) == 0 {
		return []domain.Candidate{}
	}
	byKey := make(map[string]domain.Candidate, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = enrichCandidate(candidate)
		key := candidateKey(candidate)
		if key == "" {
			key = "id:" + candidate.SourceID + ":" + candidate.ID
		}
		existing, found := byKey[key]
		if !found {
			order = append(order, key)
			byKey[key] = candidate
			continue
		}
		byKey[key] = mergeCandidate(existing, candidate)
	}
	result := make([]domain.Candidate, 0, len(order))
	for _, key := range order {
		candidate := byKey[key]
		candidate.Score = scoreCandidate(candidate)
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].SourceID != result[j].SourceID {
			return result[i].SourceID < result[j].SourceID
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func enrichCandidate(candidate domain.Candidate) domain.Candidate {
	text := strings.TrimSpace(candidate.Title + " " + candidate.Filename)
	if candidate.Resolution == "" {
		if match := resolutionPattern.FindStringSubmatch(text); len(match) > 1 {
			candidate.Resolution = normalizeResolution(match[1])
		}
	}
	if candidate.Codec == "" {
		if match := codecPattern.FindStringSubmatch(text); len(match) > 1 {
			candidate.Codec = normalizeCodec(match[1])
		}
	}
	if candidate.SizeBytes == 0 {
		if match := sizePattern.FindStringSubmatch(text); len(match) > 2 {
			if value, err := strconv.ParseFloat(match[1], 64); err == nil {
				multiplier := float64(1 << 20)
				switch strings.ToLower(match[2]) {
				case "gb":
					multiplier = 1 << 30
				case "tb":
					multiplier = 1 << 40
				}
				candidate.SizeBytes = int64(value * multiplier)
			}
		}
	}
	if candidate.Torrent != nil {
		candidate.Torrent.InfoHash = strings.ToLower(strings.TrimSpace(candidate.Torrent.InfoHash))
	}
	return candidate
}

func candidateKey(candidate domain.Candidate) string {
	if candidate.Torrent != nil && candidate.Torrent.InfoHash != "" {
		index := -1
		if candidate.Torrent.FileIndex != nil {
			index = *candidate.Torrent.FileIndex
		}
		return fmt.Sprintf("torrent:%s:%d", strings.ToLower(candidate.Torrent.InfoHash), index)
	}
	if candidate.Usenet != nil && candidate.Usenet.NZBURL != "" {
		return "usenet:" + candidate.Usenet.NZBURL
	}
	if candidate.HTTP != nil && candidate.HTTP.URL != "" {
		parsed, err := url.Parse(candidate.HTTP.URL)
		if err == nil {
			parsed.Fragment = ""
			return "http:" + parsed.String()
		}
		return "http:" + candidate.HTTP.URL
	}
	return ""
}

func mergeCandidate(left, right domain.Candidate) domain.Candidate {
	if left.Title == "" {
		left.Title = right.Title
	}
	if left.Filename == "" {
		left.Filename = right.Filename
	}
	if left.SourceName == "" {
		left.SourceName = right.SourceName
	}
	if left.UpstreamName == "" {
		left.UpstreamName = right.UpstreamName
	}
	if left.SizeBytes == 0 {
		left.SizeBytes = right.SizeBytes
	}
	if left.Resolution == "" {
		left.Resolution = right.Resolution
	}
	if left.Codec == "" {
		left.Codec = right.Codec
	}
	if len(left.Languages) == 0 {
		left.Languages = append([]string(nil), right.Languages...)
	}
	if left.Seeders == nil || (right.Seeders != nil && *right.Seeders > *left.Seeders) {
		left.Seeders = right.Seeders
	}
	if left.Cached == nil || (right.Cached != nil && *right.Cached) {
		left.Cached = right.Cached
	}
	return left
}

func scoreCandidate(candidate domain.Candidate) float64 {
	score := 0.0
	english, foreign := false, false
	for _, language := range candidate.Languages {
		switch strings.ToLower(strings.TrimSpace(language)) {
		case "en", "eng", "english":
			english = true
		default:
			foreign = true
		}
	}
	if englishPattern.MatchString(candidate.Title + " " + candidate.Filename) {
		english = true
	}
	if foreignPattern.MatchString(candidate.Title + " " + candidate.Filename) {
		foreign = true
	}
	if english {
		score += 400
	} else if foreign {
		score -= 400
	}
	if candidate.Cached != nil {
		if *candidate.Cached {
			score += 1000
		} else {
			score -= 50
		}
	}
	if candidate.HTTP != nil {
		score += 200
	}
	switch normalizeResolution(candidate.Resolution) {
	case "2160p":
		score += 160
	case "1080p":
		score += 120
	case "720p":
		score += 80
	case "480p":
		score += 40
	}
	switch normalizeCodec(candidate.Codec) {
	case "av1":
		score += 30
	case "hevc":
		score += 20
	case "h264":
		score += 10
	}
	if candidate.Seeders != nil {
		seeders := *candidate.Seeders
		if seeders > 100 {
			seeders = 100
		}
		if seeders > 0 {
			score += float64(seeders)
		}
	}
	return score
}

func normalizeResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "4k", "2160p":
		return "2160p"
	case "1080p", "720p", "480p":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeCodec(value string) string {
	value = strings.ToLower(strings.NewReplacer(".", "", "_", "", "-", "", " ", "").Replace(value))
	switch value {
	case "av1":
		return "av1"
	case "hevc", "h265", "x265":
		return "hevc"
	case "h264", "x264":
		return "h264"
	default:
		return value
	}
}
