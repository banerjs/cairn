// Package recovery writes operator recovery artifacts (FORMAT.md, hints, public recipients).
package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banerjs/cairn/format"
	appcfg "github.com/banerjs/cairn/internal/config"
)

// ExportRecoveryKit writes an offline-oriented directory:
//   - FORMAT.md       — full format spec (embedded from repo)
//   - RESTORE.txt     — human steps + optional bucket/host hints
//   - recipients_public.txt — present when cfg is non-nil (one armored line per recipient)
//
// It never writes identity secrets or AWS credentials.
func ExportRecoveryKit(dir string, cfg *appcfg.Config) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("recovery kit: output directory required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("recovery kit: mkdir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "FORMAT.md"), []byte(format.Markdown), 0o600); err != nil {
		return fmt.Errorf("recovery kit: write FORMAT.md: %w", err)
	}

	var b strings.Builder
	b.WriteString(`Cairn recovery kit
==================

What this is
------------
If you lose this laptop but keep your age identity (AGE-SECRET-KEY-PQ-1...) and AWS access,
you can rebuild plaintext files from S3 using any implementation that understands FORMAT.md.

Critical secrets (store separately, offline)
--------------------------------------------
- Age identity file (decrypt capability). Without it, ciphertext is unusable.
- AWS credentials with read access to the bucket (and delete if you prune).

Never commit those to git or attach them to this folder.

Restore (outline)
-----------------
1. Install cairn (or another tool that implements FORMAT.md).
2. Configure AWS credentials and the bucket/region.
3. Run: cairn snapshots --config /path/to/config.toml
4. Run: cairn restore <snapshot-id> --target /path/to/out --config /path/to/config.toml

Passphrase-protected identities: set CAIRN_PASSPHRASE for non-interactive use.

`)

	if cfg != nil {
		b.WriteString("Hints from your config (verify before relying on printouts)\n")
		b.WriteString("--------------------------------------------------------\n")
		b.WriteString(fmt.Sprintf("host_id: %q\n", cfg.HostID))
		b.WriteString(fmt.Sprintf("s3.bucket: %q\n", cfg.S3.Bucket))
		b.WriteString(fmt.Sprintf("s3.region: %q\n", cfg.S3.Region))
		b.WriteString("\nPublic recipients (safe to share; cannot decrypt alone):\n")
		for _, r := range cfg.Encryption.Recipients {
			s := strings.TrimSpace(r)
			if s != "" {
				b.WriteString("  - ")
				b.WriteString(s)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")

		var lines []string
		for _, r := range cfg.Encryption.Recipients {
			if s := strings.TrimSpace(r); s != "" {
				lines = append(lines, s)
			}
		}
		if len(lines) > 0 {
			recPath := filepath.Join(dir, "recipients_public.txt")
			if err := os.WriteFile(recPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
				return fmt.Errorf("recovery kit: write recipients_public.txt: %w", err)
			}
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "RESTORE.txt"), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("recovery kit: write RESTORE.txt: %w", err)
	}
	return nil
}
