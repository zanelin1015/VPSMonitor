package client

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"testing"
	"time"
)

func TestBuildLocalCertificateKeepsDomainCertificates(t *testing.T) {
	certFile := certificateFile{
		path:    "/etc/letsencrypt/live/example/fullchain.pem",
		dir:     "/etc/letsencrypt/live/example",
		hasCert: true,
		cert: &x509.Certificate{
			Subject:  pkix.Name{CommonName: "legacy.example.com"},
			Issuer:   pkix.Name{CommonName: "issuer"},
			DNSNames: []string{"edge.example.com", "203.0.113.10", "*.example.com"},
			NotAfter: time.Now().Add(24 * time.Hour),
		},
	}
	keyFile := certificateFile{path: "/etc/letsencrypt/live/example/privkey.pem", hasKey: true}

	entry := buildLocalCertificate(certFile, keyFile, certFile.dir)
	want := []string{"edge.example.com", "*.example.com", "legacy.example.com"}
	if len(entry.DNSNames) != len(want) {
		t.Fatalf("expected %d domain names, got %#v", len(want), entry.DNSNames)
	}
	for index, value := range want {
		if entry.DNSNames[index] != value {
			t.Fatalf("expected domain %q at %d, got %#v", value, index, entry.DNSNames)
		}
	}
}

func TestPairCertificateFilesFiltersIPOnlyCertificates(t *testing.T) {
	files := []certificateFile{
		{
			path:    "/etc/ssl/ip/fullchain.pem",
			dir:     "/etc/ssl/ip",
			hasCert: true,
			cert: &x509.Certificate{
				Subject:     pkix.Name{CommonName: "203.0.113.10"},
				IPAddresses: []net.IP{net.ParseIP("203.0.113.10")},
				NotAfter:    time.Now().Add(24 * time.Hour),
			},
		},
		{path: "/etc/ssl/ip/privkey.pem", dir: "/etc/ssl/ip", hasKey: true},
		{
			path:    "/etc/ssl/domain/fullchain.pem",
			dir:     "/etc/ssl/domain",
			hasCert: true,
			cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "domain.example.com"},
				DNSNames: []string{"domain.example.com"},
				NotAfter: time.Now().Add(24 * time.Hour),
			},
		},
		{path: "/etc/ssl/domain/privkey.pem", dir: "/etc/ssl/domain", hasKey: true},
	}

	inventory := pairCertificateFiles(files)
	if len(inventory) != 1 {
		t.Fatalf("expected only domain certificate, got %#v", inventory)
	}
	if got := inventory[0].DNSNames[0]; got != "domain.example.com" {
		t.Fatalf("unexpected certificate domain %q", got)
	}
}
