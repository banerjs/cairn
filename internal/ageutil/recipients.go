// Package ageutil parses post-quantum age recipients and identities for cairn v1.
package ageutil

import (
	"fmt"
	"os"
	"strings"

	"filippo.io/age"
	"filippo.io/age/tag"
)

// ParsePQRecipients parses recipient strings; only hybrid PQ (age1pq1) and tagged PQ (age1tagpq1) are allowed.
func ParsePQRecipients(lines []string) ([]age.Recipient, error) {
	var out []age.Recipient
	for i, raw := range lines {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "ssh-") {
			return nil, fmt.Errorf("recipient %d: SSH recipients are not supported in cairn v1 (use age1pq1...)", i)
		}
		switch {
		case strings.HasPrefix(s, "age1tag"):
			r, err := tag.ParseRecipient(s)
			if err != nil {
				return nil, fmt.Errorf("recipient %d: %w", i, err)
			}
			out = append(out, r)
		case strings.HasPrefix(s, "age1pq1"):
			r, err := age.ParseHybridRecipient(s)
			if err != nil {
				return nil, fmt.Errorf("recipient %d: %w", i, err)
			}
			out = append(out, r)
		case strings.HasPrefix(s, "age1"):
			return nil, fmt.Errorf("recipient %d: classical X25519 recipients (age1...) are rejected; use age1pq1 recipients instead", i)
		default:
			return nil, fmt.Errorf("recipient %d: unknown recipient format", i)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no recipients configured")
	}
	return out, nil
}

// LoadIdentities reads an age identity file (AGE-SECRET-KEY-1 or AGE-SECRET-KEY-PQ-1 lines).
func LoadIdentities(path string) ([]age.Identity, error) {
	// #nosec G304 -- path comes from explicit configuration or CAIRN_IDENTITY_FILE
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("identity file: %w", err)
	}
	defer func() { _ = f.Close() }()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("identity file: %w", err)
	}
	return ids, nil
}

// IdentityPath resolves the identity file path: CAIRN_IDENTITY_FILE overrides config path.
func IdentityPath(envVal, configPath string) (string, error) {
	if strings.TrimSpace(envVal) != "" {
		return strings.TrimSpace(envVal), nil
	}
	if strings.TrimSpace(configPath) != "" {
		return strings.TrimSpace(configPath), nil
	}
	return "", fmt.Errorf("identity path not set (CAIRN_IDENTITY_FILE or encryption.identity_file)")
}

// GeneratePQIdentity creates a new hybrid identity and returns armored secret + public recipient line.
func GeneratePQIdentity() (identityLine string, recipientLine string, err error) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		return "", "", err
	}
	return id.String(), id.Recipient().String(), nil
}
