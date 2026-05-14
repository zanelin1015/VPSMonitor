package client

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"bridge-core/internal/model"
)

const (
	certificateScanCacheTTL = 15 * time.Minute
	maxCertificateWalkDepth = 4
	maxCertificateFiles     = 256
)

type certificateFile struct {
	path     string
	dir      string
	baseName string
	hasCert  bool
	hasKey   bool
	cert     *x509.Certificate
}

func (a *App) localCertificates() []model.XUILocalCertificate {
	if !a.certsScannedAt.IsZero() && time.Since(a.certsScannedAt) < certificateScanCacheTTL {
		return append([]model.XUILocalCertificate(nil), a.certificates...)
	}

	inventory := scanLocalCertificates()
	a.certificates = inventory
	a.certsScannedAt = time.Now().UTC()
	return append([]model.XUILocalCertificate(nil), inventory...)
}

func scanLocalCertificates() []model.XUILocalCertificate {
	roots := defaultCertificateRoots()
	files := make([]certificateFile, 0, 32)

	for _, root := range roots {
		stat, err := os.Stat(root)
		if err != nil || !stat.IsDir() {
			continue
		}
		root = filepath.Clean(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				if certificateWalkDepth(root, path) > maxCertificateWalkDepth {
					return fs.SkipDir
				}
				return nil
			}
			if len(files) >= maxCertificateFiles {
				return fs.SkipAll
			}

			file, ok := inspectCertificateFile(path)
			if ok {
				files = append(files, file)
			}
			return nil
		})
		if len(files) >= maxCertificateFiles {
			break
		}
	}

	inventory := pairCertificateFiles(files)
	sort.Slice(inventory, func(i, j int) bool {
		left := inventory[i]
		right := inventory[j]
		leftExpiry := time.Time{}
		rightExpiry := time.Time{}
		if left.NotAfter != nil {
			leftExpiry = *left.NotAfter
		}
		if right.NotAfter != nil {
			rightExpiry = *right.NotAfter
		}
		if !leftExpiry.Equal(rightExpiry) {
			if left.NotAfter == nil {
				return false
			}
			if right.NotAfter == nil {
				return true
			}
			return leftExpiry.Before(rightExpiry)
		}
		return left.Name < right.Name
	})
	return inventory
}

func defaultCertificateRoots() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\cert`,
			`C:\certs`,
			`C:\x-ui`,
			`C:\xray`,
			`C:\ProgramData\Xray`,
			`C:\ProgramData\acme`,
		}
	}

	return []string{
		"/etc/letsencrypt/live",
		"/etc/ssl",
		"/etc/xray",
		"/usr/local/etc/xray",
		"/etc/nginx/ssl",
		"/www/server/panel/vhost/cert",
		"/root/cert",
		"/etc/v2ray-agent/tls",
	}
}

func certificateWalkDepth(root, path string) int {
	if path == root {
		return 0
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}

func inspectCertificateFile(path string) (certificateFile, bool) {
	lower := strings.ToLower(filepath.Base(path))
	if !looksLikeCertificateFile(lower) {
		return certificateFile{}, false
	}

	content, err := os.ReadFile(path)
	if err != nil || len(content) == 0 {
		return certificateFile{}, false
	}

	file := certificateFile{
		path:     path,
		dir:      filepath.Dir(path),
		baseName: normalizeCertificateBase(filepath.Base(path)),
	}

	var block *pem.Block
	rest := content
	for len(rest) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		blockType := strings.ToUpper(block.Type)
		if strings.Contains(blockType, "CERTIFICATE") && file.cert == nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				file.cert = cert
			}
			file.hasCert = true
			continue
		}
		if strings.Contains(blockType, "PRIVATE KEY") {
			file.hasKey = true
		}
	}

	if !file.hasCert && !file.hasKey {
		if strings.HasSuffix(lower, ".key") || strings.Contains(lower, "privkey") {
			file.hasKey = true
		}
	}
	if !file.hasCert && !file.hasKey {
		return certificateFile{}, false
	}
	return file, true
}

func looksLikeCertificateFile(name string) bool {
	switch {
	case strings.HasSuffix(name, ".crt"),
		strings.HasSuffix(name, ".cer"),
		strings.HasSuffix(name, ".pem"),
		strings.HasSuffix(name, ".key"),
		strings.Contains(name, "fullchain"),
		strings.Contains(name, "privkey"),
		strings.Contains(name, "certificate"):
		return true
	default:
		return false
	}
}

func normalizeCertificateBase(name string) string {
	lower := strings.ToLower(name)
	for _, token := range []string{
		".pem", ".crt", ".cer", ".key",
		"fullchain", "privkey", "certificate", "cert", "chain", "key",
	} {
		lower = strings.ReplaceAll(lower, token, "")
	}
	lower = strings.Trim(lower, "-_. ")
	if lower == "" {
		return strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	}
	return lower
}

func pairCertificateFiles(files []certificateFile) []model.XUILocalCertificate {
	byDir := make(map[string][]certificateFile)
	for _, file := range files {
		byDir[file.dir] = append(byDir[file.dir], file)
	}

	var inventory []model.XUILocalCertificate
	seen := make(map[string]struct{})
	for dir, items := range byDir {
		certs := make([]certificateFile, 0, len(items))
		keys := make([]certificateFile, 0, len(items))
		for _, item := range items {
			if item.hasCert {
				certs = append(certs, item)
			}
			if item.hasKey {
				keys = append(keys, item)
			}
		}
		if len(certs) == 0 || len(keys) == 0 {
			continue
		}

		for _, certFile := range certs {
			keyFile, ok := findMatchingKey(certFile, keys)
			if !ok {
				continue
			}
			entry := buildLocalCertificate(certFile, keyFile, dir)
			if entry.ID == "" {
				continue
			}
			if len(entry.DNSNames) == 0 {
				continue
			}
			if _, exists := seen[entry.ID]; exists {
				continue
			}
			seen[entry.ID] = struct{}{}
			inventory = append(inventory, entry)
		}
	}
	return inventory
}

func findMatchingKey(certFile certificateFile, keys []certificateFile) (certificateFile, bool) {
	if certFile.hasKey {
		return certFile, true
	}
	if len(keys) == 1 {
		return keys[0], true
	}
	for _, keyFile := range keys {
		if keyFile.baseName != "" && keyFile.baseName == certFile.baseName {
			return keyFile, true
		}
	}
	for _, keyFile := range keys {
		keyName := strings.ToLower(filepath.Base(keyFile.path))
		if strings.Contains(keyName, "privkey") {
			return keyFile, true
		}
	}
	return certificateFile{}, false
}

func buildLocalCertificate(certFile, keyFile certificateFile, dir string) model.XUILocalCertificate {
	sum := sha1.Sum([]byte(certFile.path + "\n" + keyFile.path))
	entry := model.XUILocalCertificate{
		ID:        hex.EncodeToString(sum[:]),
		Name:      filepath.Base(dir),
		SourceDir: dir,
		CertPath:  certFile.path,
		KeyPath:   keyFile.path,
	}
	if entry.Name == "." || entry.Name == string(os.PathSeparator) {
		entry.Name = filepath.Base(certFile.path)
	}
	if certFile.cert != nil {
		entry.Subject = certFile.cert.Subject.CommonName
		entry.Issuer = certFile.cert.Issuer.CommonName
		entry.DNSNames = certificateDomainNames(certFile.cert)
		entry.NotAfter = ptrTime(certFile.cert.NotAfter.UTC())
		if entry.Name == "" {
			entry.Name = firstNonEmpty(certFile.cert.Subject.CommonName, filepath.Base(certFile.path))
		}
	}
	if entry.Name == "" {
		entry.Name = filepath.Base(certFile.path)
	}
	return entry
}

func certificateDomainNames(cert *x509.Certificate) []string {
	if cert == nil {
		return nil
	}
	values := append([]string{}, cert.DNSNames...)
	values = append(values, cert.Subject.CommonName)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		domain := normalizeCertificateDomain(value)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	return result
}

func normalizeCertificateDomain(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, ".")
	if strings.HasPrefix(value, "*.") {
		suffix := strings.TrimPrefix(value, "*.")
		if suffix == "" || net.ParseIP(suffix) != nil || strings.Contains(suffix, " ") {
			return ""
		}
		return "*." + suffix
	}
	if value == "" || strings.Contains(value, " ") || strings.Contains(value, ":") {
		return ""
	}
	if net.ParseIP(value) != nil {
		return ""
	}
	return value
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
