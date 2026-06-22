package semantic

import (
	"bytes"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type SkipReason string

const (
	SkipReasonBinary     SkipReason = "binary"
	SkipReasonGenerated  SkipReason = "generated"
	SkipReasonVendor     SkipReason = "vendor"
	SkipReasonDependency SkipReason = "dependency"
	SkipReasonLarge      SkipReason = "large"
	SkipReasonSecret     SkipReason = "secret"
	SkipReasonReadError  SkipReason = "read-error"
)

type SkipDecision struct {
	Skip    bool
	Reason  SkipReason
	Details string
}

type FilterOptions struct {
	MaxFileBytes    int64
	SecretScanBytes int
}

const (
	defaultMaxFileBytes    int64 = 512 * 1024
	defaultSecretScanBytes       = 8 * 1024
)

type FileFilter struct {
	maxFileBytes    int64
	secretScanBytes int
}

func NewFileFilter(opts FilterOptions) FileFilter {
	maxFileBytes := opts.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxFileBytes
	}
	secretScanBytes := opts.SecretScanBytes
	if secretScanBytes <= 0 {
		secretScanBytes = defaultSecretScanBytes
	}
	return FileFilter{
		maxFileBytes:    maxFileBytes,
		secretScanBytes: secretScanBytes,
	}
}

func (f FileFilter) ShouldSkipPath(relPath string, size int64) SkipDecision {
	relPath = normalizeRelPath(relPath)
	if relPath == "" {
		return SkipDecision{}
	}
	if size > f.maxFileBytes {
		return SkipDecision{Skip: true, Reason: SkipReasonLarge, Details: "exceeds max file bytes"}
	}
	segments := strings.Split(relPath, "/")
	for _, seg := range segments {
		switch strings.ToLower(seg) {
		case "vendor":
			return SkipDecision{Skip: true, Reason: SkipReasonVendor, Details: "under vendor directory"}
		case "node_modules", ".venv", "venv", "dist", "build", "target", "out", ".next", ".nuxt", ".turbo", "coverage":
			return SkipDecision{Skip: true, Reason: SkipReasonDependency, Details: "under dependency/build directory"}
		}
	}
	name := strings.ToLower(filepath.Base(relPath))
	if strings.HasPrefix(name, ".env") && name != ".env.example" {
		return SkipDecision{Skip: true, Reason: SkipReasonSecret, Details: "secret-like env filename"}
	}
	switch name {
	case "id_rsa", "id_ed25519", "id_ecdsa", "credentials.json", "service-account.json", "serviceaccount.json":
		return SkipDecision{Skip: true, Reason: SkipReasonSecret, Details: "secret-like credential filename"}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pem", ".key", ".p12", ".pfx", ".kdbx", ".jks", ".keystore":
		return SkipDecision{Skip: true, Reason: SkipReasonSecret, Details: "secret-like key/cert filename"}
	}
	if isLikelyGeneratedName(name) {
		return SkipDecision{Skip: true, Reason: SkipReasonGenerated, Details: "generated filename"}
	}
	return SkipDecision{}
}

func (f FileFilter) ShouldSkipContent(relPath string, body []byte) SkipDecision {
	if len(body) == 0 {
		return SkipDecision{}
	}
	sample := body
	if len(sample) > f.secretScanBytes {
		sample = sample[:f.secretScanBytes]
	}
	if looksBinary(sample) {
		return SkipDecision{Skip: true, Reason: SkipReasonBinary, Details: "binary bytes detected"}
	}
	if hasGeneratedMarker(sample) {
		return SkipDecision{Skip: true, Reason: SkipReasonGenerated, Details: "generated marker detected"}
	}
	if hasSecretLikeContent(sample) {
		return SkipDecision{Skip: true, Reason: SkipReasonSecret, Details: "secret-like content marker detected"}
	}
	return SkipDecision{}
}

func isLikelyGeneratedName(name string) bool {
	if strings.HasSuffix(name, ".pb.go") ||
		strings.HasSuffix(name, ".gen.go") ||
		strings.HasSuffix(name, "_generated.go") ||
		strings.HasSuffix(name, ".generated.go") ||
		strings.HasSuffix(name, ".min.js") ||
		strings.HasSuffix(name, ".min.css") {
		return true
	}
	switch name {
	case "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb":
		return true
	}
	return false
}

func looksBinary(sample []byte) bool {
	if bytes.IndexByte(sample, 0) >= 0 {
		return true
	}
	if utf8.Valid(sample) {
		return false
	}
	nonText := 0
	for _, b := range sample {
		if b == '\n' || b == '\r' || b == '\t' || b == '\f' || b == '\b' {
			continue
		}
		if b < 0x20 || b > 0x7E {
			nonText++
		}
	}
	return len(sample) > 0 && float64(nonText)/float64(len(sample)) > 0.30
}

func hasGeneratedMarker(sample []byte) bool {
	s := strings.ToLower(string(sample))
	if strings.Contains(s, "code generated") && strings.Contains(s, "do not edit") {
		return true
	}
	return strings.Contains(s, "@generated")
}

func hasSecretLikeContent(sample []byte) bool {
	s := strings.ToLower(string(sample))
	markers := []string{
		"-----begin private key-----",
		"-----begin rsa private key-----",
		"-----begin openssh private key-----",
		"aws_secret_access_key",
		"x-api-key",
		"authorization: bearer ",
		"github_pat_",
		"ghp_",
		"sk-live-",
		"sk_test_",
		"postgres://",
		"mongodb+srv://",
	}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	// Heuristic key assignment detector to catch obvious inline credentials.
	lines := strings.Split(s, "\n")
	for i := 0; i < len(lines) && i < 120; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "password") || strings.Contains(line, "secret") || strings.Contains(line, "token") {
			if idx := strings.IndexByte(line, '='); idx > 0 {
				val := strings.Trim(strings.TrimSpace(line[idx+1:]), `"'`)
				if len(val) >= 16 && hasLettersAndDigits(val) {
					return true
				}
			}
		}
	}
	return false
}

func hasLettersAndDigits(s string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}
