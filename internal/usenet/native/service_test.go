package native

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"media-engine/internal/domain"
	"media-engine/internal/secretbox"
)

func TestNativeNNTPStreamRange(t *testing.T) {
	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	messageID := "segment@example.test"

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		_, _ = writer.WriteString("200 mock nntp ready\r\n")
		_ = writer.Flush()

		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "AUTHINFO USER "):
				_, _ = writer.WriteString("381 password required\r\n")
			case strings.HasPrefix(line, "AUTHINFO PASS "):
				_, _ = writer.WriteString("281 authentication accepted\r\n")
			case strings.HasPrefix(line, "BODY "):
				_, _ = writer.WriteString("222 0 <" + messageID + "> body follows\r\n")
				_, _ = writer.WriteString("=ybegin line=128 size=" + fmt.Sprint(len(payload)) + " name=movie.mkv\r\n")
				encoded := encodeYEncForTest(payload)
				_, _ = writer.Write(encoded)
				_, _ = writer.WriteString("\r\n=yend size=" + fmt.Sprint(len(payload)) + "\r\n.\r\n")
			default:
				_, _ = writer.WriteString("500 unsupported\r\n")
			}
			_ = writer.Flush()
		}
	}()

	nzbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-nzb")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <file poster="x" date="0" subject="&quot;movie.mkv&quot; yEnc (1/1)">
    <groups><group>alt.binaries.test</group></groups>
    <segments><segment bytes="%d" number="1">%s</segment></segments>
  </file>
</nzb>`, len(payload), messageID)
	}))
	defer nzbServer.Close()

	box, err := secretbox.NewFromHex(strings.Repeat("11", 32))
	if err != nil {
		t.Fatalf("NewFromHex: %v", err)
	}
	service, err := NewService(box, nzbServer.Client(), t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var port int
	if _, err := fmt.Sscan(portText, &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	credential, _ := json.Marshal([]Server{{
		Username: "user", Password: "pass", Host: host, Port: port, SSL: false, Connections: 1,
	}})
	resolver, err := NewResolver("native", string(credential), service)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	candidate, err := resolver.Resolve(context.Background(), domain.Candidate{
		Kind:   domain.CandidateUsenet,
		Usenet: &domain.UsenetInfo{NZBURL: nzbServer.URL + "/movie.nzb"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if candidate.HTTP == nil {
		t.Fatal("expected stream URL")
	}
	token := strings.TrimPrefix(candidate.HTTP.URL, "/api/v1/usenet/stream/")
	if token == candidate.HTTP.URL || token == "" {
		t.Fatalf("unexpected stream path: %s", candidate.HTTP.URL)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usenet/stream/"+token, nil)
	req.Header.Set("Range", "bytes=5-12")
	rec := httptest.NewRecorder()
	service.ServeToken(rec, req, token)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got, want := rec.Body.String(), string(payload[5:13]); got != want {
		t.Fatalf("range body = %q, want %q", got, want)
	}

	_ = listener.Close()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("NNTP mock did not stop")
	}
}

func TestDecodeYEncEscapes(t *testing.T) {
	input := []byte{0, 13, 10, '=', 255}
	decoded, err := decodeYEnc([][]byte{[]byte("=ybegin line=128 size=5 name=x.bin"), encodeYEncForTest(input), []byte("=yend size=5")})
	if err != nil {
		t.Fatalf("decodeYEnc: %v", err)
	}
	if string(decoded) != string(input) {
		t.Fatalf("decoded = %v, want %v", decoded, input)
	}
}

func encodeYEncForTest(input []byte) []byte {
	output := make([]byte, 0, len(input)*2)
	for _, raw := range input {
		value := raw + 42
		if value == 0 || value == '\n' || value == '\r' || value == '=' || value == '.' {
			output = append(output, '=')
			value += 64
		}
		output = append(output, value)
	}
	return output
}
