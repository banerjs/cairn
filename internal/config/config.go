// Package config loads and validates cairn.toml configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var hostIDRe = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// hostnameForConfig is swapped in tests (default host_id from OS hostname).
var hostnameForConfig = os.Hostname
var userHomeDirForConfig = os.UserHomeDir
var getenvForConfig = os.Getenv
var goosForConfig = func() string { return runtime.GOOS }

var allowedStorageClasses = map[string]struct{}{
	"STANDARD":            {},
	"STANDARD_IA":         {},
	"REDUCED_REDUNDANCY":  {},
	"ONEZONE_IA":          {},
	"INTELLIGENT_TIERING": {},
	"GLACIER":             {},
	"DEEP_ARCHIVE":        {},
	"GLACIER_IR":          {},
	"OUTPOSTS":            {},
}

// Config is the full operator configuration for backup and restore-side commands.
type Config struct {
	HostID       string           `toml:"host_id"`
	CleanupGrace string           `toml:"cleanup_grace"`
	S3           S3Config         `toml:"s3"`
	Encryption   EncryptionConfig `toml:"encryption"`
	Backup       BackupConfig     `toml:"backup"`

	cleanupDur time.Duration
}

// S3Config holds bucket settings (credentials come from the AWS SDK default chain).
type S3Config struct {
	Bucket       string `toml:"bucket"`
	Region       string `toml:"region"`
	StorageClass string `toml:"storage_class"`
}

// EncryptionConfig holds age recipients and optional identity path for decrypt commands.
type EncryptionConfig struct {
	Recipients   []string `toml:"recipients"`
	IdentityFile string   `toml:"identity_file"`
}

// BackupConfig describes what to back up and ignore rules.
type BackupConfig struct {
	SourceRoots    []string `toml:"source_roots"`
	Parallelism    int      `toml:"parallelism"`
	FollowSymlinks *bool    `toml:"follow_symlinks"`
	Excludes       []string `toml:"excludes"`
	Includes       []string `toml:"includes"`
}

// FollowSymlinksEffective returns true unless explicitly set false in config.
func (c *Config) FollowSymlinksEffective() bool {
	if c.Backup.FollowSymlinks == nil {
		return true
	}
	return *c.Backup.FollowSymlinks
}

// CleanupGraceDuration returns parsed cleanup grace interval.
func (c *Config) CleanupGraceDuration() time.Duration {
	return c.cleanupDur
}

// Load reads and validates a TOML config file after expanding ${VAR} placeholders.
func Load(path string) (*Config, error) {
	// #nosec G304 -- config path is supplied explicitly by the operator
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	expanded := os.ExpandEnv(string(b))
	var c Config
	meta, err := toml.Decode(expanded, &c)
	if err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if und := meta.Undecoded(); len(und) > 0 {
		return nil, fmt.Errorf("config: unknown keys: %v", und)
	}
	if err := c.applyDefaults(); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() error {
	if c.HostID == "" {
		h, err := hostnameForConfig()
		if err != nil {
			return fmt.Errorf("config: host_id unset and hostname failed: %w", err)
		}
		slug, err := slugifyHostnameAsHostID(h)
		if err != nil {
			return fmt.Errorf("config: host_id from hostname: %w", err)
		}
		c.HostID = slug
	}
	if c.CleanupGrace == "" {
		c.CleanupGrace = "24h"
	}
	d, err := time.ParseDuration(c.CleanupGrace)
	if err != nil {
		return fmt.Errorf("config: cleanup_grace: %w", err)
	}
	c.cleanupDur = d
	if c.S3.StorageClass == "" {
		c.S3.StorageClass = "STANDARD_IA"
	}
	if c.Backup.Parallelism <= 0 {
		c.Backup.Parallelism = min(8, max(1, runtime.GOMAXPROCS(0)))
	}
	return nil
}

// slugifyHostnameAsHostID maps an OS hostname to a valid host_id (used only when
// host_id is omitted from config). It lowercases ASCII letters and replaces runs
// of other characters with a single hyphen.
func slugifyHostnameAsHostID(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("empty hostname after trim")
	}
	host = strings.ToLower(host)
	var b strings.Builder
	lastSep := false
	for _, r := range host {
		if isHostIDSlugRune(r) {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if b.Len() > 0 && !lastSep {
			b.WriteByte('-')
			lastSep = true
		}
	}
	s := b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		return "", fmt.Errorf("hostname contained no usable host_id characters")
	}
	runes := []rune(s)
	if len(runes) > 64 {
		s = string(runes[:64])
	}
	return s, nil
}

func isHostIDSlugRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.', r == '_', r == '-':
		return true
	default:
		return false
	}
}

// Validate checks mandatory fields and formats.
func (c *Config) Validate() error {
	if !hostIDRe.MatchString(c.HostID) {
		return fmt.Errorf("config: host_id must match %s", hostIDRe.String())
	}
	if strings.TrimSpace(c.S3.Bucket) == "" {
		return fmt.Errorf("config: s3.bucket is required")
	}
	if strings.TrimSpace(c.S3.Region) == "" {
		return fmt.Errorf("config: s3.region is required")
	}
	if _, ok := allowedStorageClasses[c.S3.StorageClass]; !ok {
		return fmt.Errorf("config: unsupported s3.storage_class %q", c.S3.StorageClass)
	}
	if len(c.Encryption.Recipients) == 0 {
		return fmt.Errorf("config: encryption.recipients must be non-empty")
	}
	return nil
}

// DefaultConfigPath returns the OS-default config.toml location when --config is omitted.
func DefaultConfigPath() string {
	home, _ := userHomeDirForConfig()
	switch goosForConfig() {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "cairn", "config.toml")
	case "windows":
		app := getenvForConfig("APPDATA")
		return filepath.Join(app, "cairn", "config.toml")
	default:
		xdg := getenvForConfig("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, "cairn", "config.toml")
	}
}
