package server

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const maxProxiedImageBytes = 12 << 20

func (a *App) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	parsed, err := validateImageProxyURL(rawURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image url")
		return
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialPublicImageProxyAddress,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			_, err := validateImageProxyURL(req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build image request: %v", err))
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VPSMonitor/1.0)")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/png,image/jpeg,image/gif,image/*;q=0.5")
	req.Header.Set("Referer", parsed.Scheme+"://"+parsed.Host+"/")

	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch image failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("fetch image: http %d", resp.StatusCode))
		return
	}
	contentType, err := safeProxiedImageContentType(resp.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "remote resource is not an image")
		return
	}
	if resp.ContentLength > maxProxiedImageBytes {
		writeError(w, http.StatusBadGateway, "remote image is too large")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if resp.Header.Get("Last-Modified") != "" {
		w.Header().Set("Last-Modified", resp.Header.Get("Last-Modified"))
	}
	if resp.Header.Get("ETag") != "" {
		w.Header().Set("ETag", resp.Header.Get("ETag"))
	}
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxProxiedImageBytes))
}

func isPublicImageProxyHost(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !isPublicImageProxyIP(addr) {
			return false
		}
	}
	return true
}

func validateImageProxyURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid image url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported image url scheme")
	}
	if parsed.Hostname() == "" || strings.EqualFold(parsed.Hostname(), "localhost") {
		return nil, fmt.Errorf("image url host is not allowed")
	}
	return parsed, nil
}

func dialPublicImageProxyAddress(ctx context.Context, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid image host address")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve image host")
	}
	for _, addr := range addresses {
		if !isPublicImageProxyIP(addr) {
			return nil, fmt.Errorf("image host resolves to a non-public address")
		}
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
}

func isPublicImageProxyIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicImageProxyPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var nonPublicImageProxyPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func safeProxiedImageContentType(value string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	mediaType = strings.ToLower(mediaType)
	switch mediaType {
	case "image/avif", "image/apng", "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon", "image/vnd.microsoft.icon":
		return mediaType, nil
	default:
		return "", fmt.Errorf("unsupported image content type")
	}
}
