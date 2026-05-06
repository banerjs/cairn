// Command cairn backs up and restores encrypted snapshots to Amazon S3.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/banerjs/cairn/internal/ageutil"
	"github.com/banerjs/cairn/internal/awsconfig"
	"github.com/banerjs/cairn/internal/backup"
	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/keygen"
	"github.com/banerjs/cairn/internal/prune"
	"github.com/banerjs/cairn/internal/recovery"
	"github.com/banerjs/cairn/internal/restore"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/snapshots"
	"github.com/banerjs/cairn/internal/status"
	"github.com/banerjs/cairn/internal/verifycmd"
	"github.com/banerjs/cairn/internal/version"

	"filippo.io/age"
)

func main() {
	os.Exit(run(os.Args))
}

// openStoreHook is swapped in tests to avoid live AWS calls.
var openStoreHook = defaultOpenStore

// getenv is swapped in tests for identity file resolution.
var getenv = os.Getenv

func run(argv []string) int {
	if len(argv) < 2 {
		usage()
		return 1
	}
	sub := argv[1]
	switch sub {
	case "-h", "--help", "help":
		usage()
		return 0
	case "version":
		fmt.Printf("%s %v\n", version.Name, version.Version)
		fmt.Printf("schemas: %s\n", strings.Join(version.ManifestSchemas, ", "))
		return 0
	}

	ctx := context.Background()

	switch sub {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		out := fs.String("output", "", "write new identity here")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		if *out == "" {
			fmt.Fprintln(os.Stderr, "keygen: --output required")
			return 1
		}
		rec, err := keygen.WriteNewHybridIdentity(*out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(rec)
		return 0

	case "backup":
		if len(argv) < 3 {
			fmt.Fprintln(os.Stderr, "backup: <config.toml> required")
			return 1
		}
		cfgPath := argv[2]
		fs := flag.NewFlagSet("backup", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		sc := fs.String("storage-class", "", "override [s3].storage_class")
		p := fs.Int("parallelism", 0, "workers (0 = config)")
		v := fs.Bool("v", false, "info logs")
		vv := fs.Bool("vv", false, "debug logs")
		if err := fs.Parse(argv[3:]); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cfg, err := appcfg.Load(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		recips, err := ageutil.ParsePQRecipients(cfg.Encryption.Recipients)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := backup.Run(ctx, cfg, st, recips, *sc, *p, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "restore":
		fs := flag.NewFlagSet("restore", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		cfgPath := fs.String("config", "", "config path (default OS-specific)")
		target := fs.String("target", "", "restore destination directory")
		p := fs.Int("parallelism", 0, "workers (0 = config)")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		args := fs.Args()
		if len(args) < 1 || *target == "" {
			fmt.Fprintln(os.Stderr, "restore: <snapshot-id> --target DIR required")
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = appcfg.DefaultConfigPath()
		}
		cfg, err := appcfg.Load(cp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		idPath, err := ageutil.IdentityPath(getenv("CAIRN_IDENTITY_FILE"), cfg.Encryption.IdentityFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		ids, err := ageutil.LoadIdentities(idPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := restore.Run(ctx, cfg, st, ids, args[0], filepath.Clean(*target), *p, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "snapshots":
		fs := flag.NewFlagSet("snapshots", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		cfgPath := fs.String("config", "", "")
		host := fs.String("host", "", "filter host")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = appcfg.DefaultConfigPath()
		}
		cfg, err := appcfg.Load(cp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		var ids []age.Identity
		idPath, err := ageutil.IdentityPath(getenv("CAIRN_IDENTITY_FILE"), cfg.Encryption.IdentityFile)
		if err == nil {
			ids, _ = ageutil.LoadIdentities(idPath)
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := snapshots.List(ctx, cfg, st, ids, *host, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "verify":
		fs := flag.NewFlagSet("verify", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		cfgPath := fs.String("config", "", "")
		sample := fs.Int("sample", 10, "0 = verify all regular files")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		args := fs.Args()
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "verify: <snapshot-id> required")
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = appcfg.DefaultConfigPath()
		}
		cfg, err := appcfg.Load(cp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		idPath, err := ageutil.IdentityPath(getenv("CAIRN_IDENTITY_FILE"), cfg.Encryption.IdentityFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		ids, err := ageutil.LoadIdentities(idPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := verifycmd.Run(ctx, cfg, st, ids, args[0], *sample, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "prune":
		fs := flag.NewFlagSet("prune", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		cfgPath := fs.String("config", "", "")
		keepLast := fs.Int("keep-last", 0, "")
		keepMonthly := fs.Int("keep-monthly", 0, "")
		dry := fs.Bool("dry-run", false, "")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		if *keepLast < 1 {
			fmt.Fprintln(os.Stderr, "prune: --keep-last must be >= 1")
			return 1
		}
		cp := *cfgPath
		if cp == "" {
			cp = appcfg.DefaultConfigPath()
		}
		cfg, err := appcfg.Load(cp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := prune.Run(ctx, st, cfg.HostID, *keepLast, *keepMonthly, *dry, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "export-recovery-kit":
		fs := flag.NewFlagSet("export-recovery-kit", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		outDir := fs.String("output", "", "directory to write FORMAT.md and hints")
		cfgPath := fs.String("config", "", "optional config for bucket hints and public recipients")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		if *outDir == "" {
			fmt.Fprintln(os.Stderr, "export-recovery-kit: --output DIR required")
			return 1
		}
		var cfg *appcfg.Config
		if strings.TrimSpace(*cfgPath) != "" {
			var err error
			cfg, err = appcfg.Load(strings.TrimSpace(*cfgPath))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		if err := recovery.ExportRecoveryKit(*outDir, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "wrote recovery kit to %s\n", *outDir)
		return 0

	case "status":
		fs := flag.NewFlagSet("status", flag.ExitOnError)
		fs.SetOutput(os.Stderr)
		cfgPath := fs.String("config", "", "")
		host := fs.String("host", "", "")
		showCost := fs.Bool("show-cost", false, "")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = appcfg.DefaultConfigPath()
		}
		cfg, err := appcfg.Load(cp)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		st, err := openStoreHook(ctx, cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := status.Run(ctx, st, *host, *showCost, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", sub)
		usage()
		return 1
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `cairn — encrypted backups to Amazon S3

Commands:
  backup <config.toml> [--storage-class CLASS] [--parallelism N] [-v]
  restore <snapshot-id> --target DIR [--config PATH] [--parallelism N] [-v]
  snapshots [--host HOST] [--config PATH] [-v]
  verify <snapshot-id> [--sample N] [--config PATH] [-v]
  prune --keep-last N [--keep-monthly M] [--dry-run] [--config PATH] [-v]
  status [--host HOST] [--show-cost] [--config PATH] [-v]
  export-recovery-kit --output DIR [--config PATH]
  keygen --output PATH
  version

Secrets: CAIRN_IDENTITY_FILE or encryption.identity_file (never pass identity path on CLI).
AWS: standard SDK credential chain.

`)
}

func loggerFromVerbosity(v, vv bool) *slog.Logger {
	level := slog.LevelWarn
	switch {
	case vv:
		level = slog.LevelDebug
	case v:
		level = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}

func defaultOpenStore(ctx context.Context, cfg *appcfg.Config) (*s3store.Store, error) {
	awscfg, err := awsconfig.Load(ctx, cfg.S3.Region)
	if err != nil {
		return nil, err
	}
	return s3store.New(ctx, awscfg, cfg.S3.Bucket)
}
