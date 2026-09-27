package safehttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AllowPrivate bool
	Timeout      time.Duration
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func New(config Config) *http.Client {
	if config.Timeout <= 0 {
		config.Timeout = 15 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConns = 128
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second

	if !config.AllowPrivate {
		transport.DialContext = safeDialContext
	}

	return &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect uses unsupported scheme")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errors.New("https redirect downgrade is blocked")
			}
			if !config.AllowPrivate && isUnsafeHost(req.URL.Hostname()) {
				return errors.New("redirect to private or local host is blocked")
			}
			return nil
		},
	}
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if isUnsafeHost(host) {
		return nil, errors.New("private or local outbound host is blocked")
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve outbound host: %w", err)
	}
	if len(ips) == 0 {
		return nil, errors.New("outbound host resolved to no addresses")
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, resolved := range ips {
		ip := resolved.Unmap()
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("outbound host %q resolves to blocked address %s", host, ip)
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func isUnsafeHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(host, ".")))
	if host == "" || host == "localhost" || !strings.Contains(host, ".") {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return !isPublicIP(ip.Unmap())
	}
	return false
}

func isPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	if ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func ValidatePublicURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("URL is required")
	}
	parsed, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	if parsed.URL.Scheme != "http" && parsed.URL.Scheme != "https" {
		return errors.New("URL must use http or https")
	}
	if parsed.URL.Hostname() == "" {
		return errors.New("URL must include a host")
	}
	if isUnsafeHost(parsed.URL.Hostname()) {
		return errors.New("private or local URL is blocked")
	}
	if port := parsed.URL.Port(); port != "" {
		if _, err := strconv.Atoi(port); err != nil {
			return errors.New("URL port is invalid")
		}
	}
	return nil
}
