package native

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"media-engine/internal/domain"
	"media-engine/internal/resolver/common"
	"media-engine/internal/resolver/credentialjson"
	"media-engine/internal/secretbox"
)

const maxNZBBytes = 16 << 20

type Server struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	SSL         bool   `json:"ssl"`
	Connections int    `json:"connections"`
}

type credentialEnvelope struct {
	Servers []Server `json:"servers"`
}

type tokenPayload struct {
	Servers   []Server `json:"servers"`
	NZBURL    string   `json:"nzbUrl"`
	FileIndex *int     `json:"fileIndex,omitempty"`
	Filename  string   `json:"filename,omitempty"`
}

type Service struct {
	box        *secretbox.Box
	httpClient *http.Client
	cacheDir   string
	group      singleflight.Group
}

type Resolver struct {
	id      string
	servers []Server
	service *Service
}

func NewService(box *secretbox.Box, client *http.Client, cacheDir string) (*Service, error) {
	if box == nil {
		return nil, errors.New("native NNTP requires MASTER_KEY")
	}
	if client == nil {
		return nil, errors.New("http client is required")
	}
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "media-engine-usenet")
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create usenet cache: %w", err)
	}
	return &Service{
		box: box, httpClient: client, cacheDir: cacheDir,
	}, nil
}

func NewResolver(id, credential string, service *Service) (*Resolver, error) {
	if service == nil {
		return nil, errors.New("native NNTP service is unavailable")
	}
	servers, err := parseServers(credential)
	if err != nil {
		return nil, err
	}
	return &Resolver{id: id, servers: servers, service: service}, nil
}

func (r *Resolver) ID() string { return r.id }

func (r *Resolver) Resolve(ctx context.Context, candidate domain.Candidate) (domain.Candidate, error) {
	if candidate.Usenet == nil || strings.TrimSpace(candidate.Usenet.NZBURL) == "" {
		return candidate, nil
	}
	payload := tokenPayload{
		Servers:   r.servers,
		NZBURL:    candidate.Usenet.NZBURL,
		FileIndex: candidate.Usenet.FileIndex,
		Filename:  candidate.Filename,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return candidate, err
	}
	token, err := r.service.box.SealURL(data)
	if err != nil {
		return candidate, err
	}

	cached := true
	candidate.Cached = &cached
	candidate.Kind = domain.CandidateDirect
	candidate.HTTP = &domain.HTTPStream{
		URL: "/api/v1/usenet/stream/" + token,
	}
	return candidate, nil
}

func parseServers(value string) ([]Server, error) {
	var servers []Server
	if err := credentialjson.Decode(value, &servers); err != nil || len(servers) == 0 {
		var envelope credentialEnvelope
		if envelopeErr := credentialjson.Decode(value, &envelope); envelopeErr != nil {
			if err != nil {
				return nil, err
			}
			return nil, envelopeErr
		}
		servers = envelope.Servers
	}
	if len(servers) == 0 {
		return nil, errors.New("at least one NNTP server is required")
	}
	for index := range servers {
		server := &servers[index]
		server.Host = strings.TrimSpace(server.Host)
		if server.Host == "" || server.Port <= 0 || server.Port > 65535 {
			return nil, errors.New("invalid NNTP server host or port")
		}
		if server.Connections <= 0 {
			server.Connections = 4
		}
		if server.Connections > 32 {
			server.Connections = 32
		}
	}
	return servers, nil
}

func (s *Service) ServeToken(w http.ResponseWriter, r *http.Request, token string) {
	payload, err := s.decodeToken(token)
	if err != nil {
		http.Error(w, "invalid stream token", http.StatusBadRequest)
		return
	}

	value, err, _ := s.group.Do(contentKey(payload), func() (any, error) {
		return s.materialize(r.Context(), payload)
	})
	if err != nil {
		http.Error(w, "usenet stream unavailable", http.StatusBadGateway)
		return
	}
	result, ok := value.(materialized)
	if !ok {
		http.Error(w, "usenet stream unavailable", http.StatusInternalServerError)
		return
	}

	file, err := os.Open(result.Path)
	if err != nil {
		http.Error(w, "usenet stream unavailable", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "usenet stream unavailable", http.StatusInternalServerError)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(result.Name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, result.Name, info.ModTime(), file)
}

func (s *Service) decodeToken(token string) (tokenPayload, error) {
	data, err := s.box.OpenURL(token)
	if err != nil {
		return tokenPayload{}, err
	}
	var payload tokenPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return tokenPayload{}, err
	}
	if payload.NZBURL == "" || len(payload.Servers) == 0 {
		return tokenPayload{}, errors.New("invalid usenet stream payload")
	}
	return payload, nil
}

type materialized struct {
	Path string
	Name string
}

func contentKey(payload tokenPayload) string {
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) materialize(ctx context.Context, payload tokenPayload) (materialized, error) {
	key := contentKey(payload)
	finalPath := filepath.Join(s.cacheDir, key+".media")
	metaPath := filepath.Join(s.cacheDir, key+".json")
	if info, err := os.Stat(finalPath); err == nil && info.Size() > 0 {
		name := payload.Filename
		if data, metaErr := os.ReadFile(metaPath); metaErr == nil {
			var metadata struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(data, &metadata) == nil && metadata.Name != "" {
				name = metadata.Name
			}
		}
		if name == "" {
			name = "stream.mkv"
		}
		return materialized{Path: finalPath, Name: name}, nil
	}

	nzb, err := s.fetchNZB(ctx, payload.NZBURL)
	if err != nil {
		return materialized{}, err
	}
	file, err := selectNZBFile(nzb.Files, payload.FileIndex)
	if err != nil {
		return materialized{}, err
	}

	partDir := filepath.Join(s.cacheDir, key+".parts")
	if err := os.MkdirAll(partDir, 0o700); err != nil {
		return materialized{}, err
	}
	defer os.RemoveAll(partDir)

	pools := make([]*connectionPool, 0, len(payload.Servers))
	totalConnections := 0
	for _, server := range payload.Servers {
		pool := newConnectionPool(server)
		pools = append(pools, pool)
		totalConnections += server.Connections
	}
	if totalConnections > 24 {
		totalConnections = 24
	}
	if totalConnections < 1 {
		totalConnections = 1
	}
	defer func() {
		for _, pool := range pools {
			pool.close()
		}
	}()

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(totalConnections)
	for index, segment := range file.Segments {
		index, segment := index, segment
		group.Go(func() error {
			data, fetchErr := fetchSegment(groupCtx, pools, segment, index)
			if fetchErr != nil {
				return fetchErr
			}
			partPath := filepath.Join(partDir, fmt.Sprintf("%08d.part", index))
			return os.WriteFile(partPath, data, 0o600)
		})
	}
	if err := group.Wait(); err != nil {
		return materialized{}, err
	}

	tempPath := finalPath + ".tmp"
	output, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return materialized{}, err
	}
	writeErr := func() error {
		defer output.Close()
		for index := range file.Segments {
			partPath := filepath.Join(partDir, fmt.Sprintf("%08d.part", index))
			part, err := os.Open(partPath)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, part)
			closeErr := part.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return output.Sync()
	}()
	if writeErr != nil {
		_ = os.Remove(tempPath)
		return materialized{}, writeErr
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return materialized{}, err
	}

	name := nzbFilename(file.Subject)
	if name == "" {
		name = payload.Filename
	}
	if name == "" {
		name = "stream.mkv"
	}
	meta, _ := json.Marshal(map[string]string{"name": name})
	_ = os.WriteFile(metaPath, meta, 0o600)
	return materialized{Path: finalPath, Name: name}, nil
}

func (s *Service) fetchNZB(ctx context.Context, nzbURL string) (nzbDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nzbURL, nil)
	if err != nil {
		return nzbDocument{}, err
	}
	res, err := s.httpClient.Do(req)
	if err != nil {
		return nzbDocument{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nzbDocument{}, fmt.Errorf("NZB URL returned HTTP %d", res.StatusCode)
	}
	var document nzbDocument
	if err := xml.NewDecoder(io.LimitReader(res.Body, maxNZBBytes)).Decode(&document); err != nil {
		return nzbDocument{}, fmt.Errorf("parse NZB: %w", err)
	}
	for index := range document.Files {
		sort.Slice(document.Files[index].Segments, func(i, j int) bool {
			return document.Files[index].Segments[i].Number < document.Files[index].Segments[j].Number
		})
	}
	return document, nil
}

type nzbDocument struct {
	Files []nzbFile `xml:"file"`
}

type nzbFile struct {
	Subject  string       `xml:"subject,attr"`
	Groups   []string     `xml:"groups>group"`
	Segments []nzbSegment `xml:"segments>segment"`
}

type nzbSegment struct {
	Bytes     int64  `xml:"bytes,attr"`
	Number    int    `xml:"number,attr"`
	MessageID string `xml:",chardata"`
}

var quotedFilename = regexp.MustCompile(`"([^"]+)"`)

func selectNZBFile(files []nzbFile, fileIndex *int) (nzbFile, error) {
	if fileIndex != nil {
		if *fileIndex < 0 || *fileIndex >= len(files) {
			return nzbFile{}, errors.New("NZB file index is out of range")
		}
		return files[*fileIndex], nil
	}

	bestIndex := -1
	var bestSize int64
	for index, file := range files {
		name := nzbFilename(file.Subject)
		if !common.IsVideo(name) {
			continue
		}
		size := nzbFileSize(file)
		if bestIndex == -1 || size > bestSize {
			bestIndex, bestSize = index, size
		}
	}
	if bestIndex < 0 {
		return nzbFile{}, errors.New("NZB contains no directly posted video file")
	}
	return files[bestIndex], nil
}

func nzbFilename(subject string) string {
	matches := quotedFilename.FindAllStringSubmatch(subject, -1)
	for index := len(matches) - 1; index >= 0; index-- {
		if len(matches[index]) > 1 && filepath.Ext(matches[index][1]) != "" {
			return matches[index][1]
		}
	}
	fields := strings.Fields(subject)
	for _, field := range fields {
		field = strings.Trim(field, "\"'()[]")
		if common.IsVideo(field) {
			return field
		}
	}
	return ""
}

func nzbFileSize(file nzbFile) int64 {
	var total int64
	for _, segment := range file.Segments {
		total += segment.Bytes
	}
	return total
}

type nntpConn struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer
}

func dialNNTP(ctx context.Context, server Server) (*nntpConn, error) {
	address := net.JoinHostPort(server.Host, strconv.Itoa(server.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if server.SSL {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			ServerName: server.Host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}
	client := &nntpConn{conn: conn, r: bufio.NewReaderSize(conn, 64<<10), w: bufio.NewWriterSize(conn, 16<<10)}
	code, _, err := client.readResponse()
	if err != nil || (code != 200 && code != 201) {
		conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("NNTP greeting returned %d", code)
	}
	if server.Username != "" {
		code, _, err = client.command("AUTHINFO USER " + server.Username)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if code == 381 {
			code, _, err = client.command("AUTHINFO PASS " + server.Password)
			if err != nil {
				conn.Close()
				return nil, err
			}
		}
		if code != 281 {
			conn.Close()
			return nil, fmt.Errorf("NNTP authentication returned %d", code)
		}
	}
	return client, nil
}

func (c *nntpConn) command(command string) (int, string, error) {
	if _, err := c.w.WriteString(command + "\r\n"); err != nil {
		return 0, "", err
	}
	if err := c.w.Flush(); err != nil {
		return 0, "", err
	}
	return c.readResponse()
}

func (c *nntpConn) readResponse() (int, string, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return 0, "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) < 3 {
		return 0, line, errors.New("invalid NNTP response")
	}
	code, err := strconv.Atoi(line[:3])
	return code, line, err
}

func (c *nntpConn) body(messageID string) ([]byte, error) {
	messageID = strings.TrimSpace(messageID)
	if !strings.HasPrefix(messageID, "<") {
		messageID = "<" + messageID + ">"
	}
	code, _, err := c.command("BODY " + messageID)
	if err != nil {
		return nil, err
	}
	if code != 222 {
		return nil, fmt.Errorf("NNTP BODY returned %d", code)
	}

	lines := make([][]byte, 0, 128)
	for {
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytesTrimCRLF(line)
		if string(line) == "." {
			break
		}
		if len(line) > 1 && line[0] == '.' && line[1] == '.' {
			line = line[1:]
		}
		copyLine := append([]byte(nil), line...)
		lines = append(lines, copyLine)
	}
	return decodeYEnc(lines)
}

func bytesTrimCRLF(value []byte) []byte {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}

func decodeYEnc(lines [][]byte) ([]byte, error) {
	total := 0
	for _, line := range lines {
		if len(line) == 0 || line[0] == '=' {
			continue
		}
		total += len(line)
	}
	output := make([]byte, 0, total)
	foundData := false
	for _, line := range lines {
		if strings.HasPrefix(string(line), "=ybegin") ||
			strings.HasPrefix(string(line), "=ypart") ||
			strings.HasPrefix(string(line), "=yend") {
			continue
		}
		if len(line) == 0 {
			continue
		}
		foundData = true
		for index := 0; index < len(line); index++ {
			value := line[index]
			if value == '=' {
				index++
				if index >= len(line) {
					return nil, errors.New("invalid yEnc escape")
				}
				value = line[index] - 64
			}
			output = append(output, value-42)
		}
	}
	if !foundData {
		return nil, errors.New("NNTP article contained no yEnc data")
	}
	return output, nil
}

type connectionPool struct {
	server  Server
	conns   chan *nntpConn
	mu      sync.Mutex
	created int
}

func newConnectionPool(server Server) *connectionPool {
	return &connectionPool{server: server, conns: make(chan *nntpConn, server.Connections)}
}

func (p *connectionPool) get(ctx context.Context) (*nntpConn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
	}

	p.mu.Lock()
	if p.created < cap(p.conns) {
		p.created++
		p.mu.Unlock()
		conn, err := dialNNTP(ctx, p.server)
		if err != nil {
			p.mu.Lock()
			p.created--
			p.mu.Unlock()
			return nil, err
		}
		return conn, nil
	}
	p.mu.Unlock()

	select {
	case conn := <-p.conns:
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *connectionPool) put(conn *nntpConn, healthy bool) {
	if conn == nil {
		return
	}
	if !healthy {
		_ = conn.conn.Close()
		p.mu.Lock()
		p.created--
		p.mu.Unlock()
		return
	}
	select {
	case p.conns <- conn:
	default:
		_ = conn.conn.Close()
		p.mu.Lock()
		p.created--
		p.mu.Unlock()
	}
}

func (p *connectionPool) close() {
	for {
		select {
		case conn := <-p.conns:
			_ = conn.conn.Close()
		default:
			return
		}
	}
}

func fetchSegment(ctx context.Context, pools []*connectionPool, segment nzbSegment, index int) ([]byte, error) {
	var lastErr error
	for offset := 0; offset < len(pools); offset++ {
		pool := pools[(index+offset)%len(pools)]
		conn, err := pool.get(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		data, err := conn.body(segment.MessageID)
		pool.put(conn, err == nil)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no NNTP providers available")
	}
	return nil, lastErr
}
