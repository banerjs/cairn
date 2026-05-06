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

// osExit is swapped in tests; production delegates to os.Exit.
var osExit = os.Exit

func defaultExit(code int) {
	osExit(code)
}

// exitHook is swapped in tests so main() yields without terminating the process.
var exitHook = defaultExit

func main() {
	exitHook(run(os.Args))
}

// openStoreHook is swapped in tests to avoid live AWS calls.
var openStoreHook = defaultOpenStore

// getenv is swapped in tests for identity file resolution.
var getenv = os.Getenv

// awsLoadForStore wraps awsconfig.Load for tests that must avoid touching the SDK.
var awsLoadForStore = awsconfig.Load

// newS3Store wraps s3store.New for deterministic tests (e.g. empty bucket error after fake AWS config).
var newS3Store = s3store.New

// defaultConfigPath resolves the config.toml path when CLI --config is empty.
var defaultConfigPath = appcfg.DefaultConfigPath

var (
	keygenRunHook         = keygen.WriteNewHybridIdentity
	backupRunHook         = backup.Run
	restoreRunHook        = restore.Run
	snapshotsListHook     = snapshots.List
	verifyRunHook         = verifycmd.Run
	pruneRunHook          = prune.Run
	statusRunHook         = status.Run
	exportRecoveryKitHook = recovery.ExportRecoveryKit
)

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

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
		fs := newFlagSet("keygen")
		out := fs.String("output", "", "write new identity here")
		if err := fs.Parse(argv[2:]); err != nil {
			return 1
		}
		if *out == "" {
			fmt.Fprintln(os.Stderr, "keygen: --output required")
			return 1
		}
		rec, err := keygenRunHook(*out)
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
		fs := newFlagSet("backup")
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
		if err := backupRunHook(ctx, cfg, st, recips, *sc, *p, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "restore":
		fs := newFlagSet("restore")
		cfgPath := fs.String("config", "", "config path (default OS-specific)")
		target := fs.String("target", "", "restore destination directory")
		p := fs.Int("parallelism", 0, "workers (0 = config)")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedRestoreFlags)); err != nil {
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
			cp = defaultConfigPath()
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
		if err := restoreRunHook(ctx, cfg, st, ids, args[0], filepath.Clean(*target), *p, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "snapshots":
		fs := newFlagSet("snapshots")
		cfgPath := fs.String("config", "", "")
		host := fs.String("host", "", "filter host")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedSnapshotsFlags)); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = defaultConfigPath()
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
		if err := snapshotsListHook(ctx, cfg, st, ids, *host, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "verify":
		fs := newFlagSet("verify")
		cfgPath := fs.String("config", "", "")
		sample := fs.Int("sample", 10, "0 = verify all regular files")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedVerifyFlags)); err != nil {
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
			cp = defaultConfigPath()
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
		if err := verifyRunHook(ctx, cfg, st, ids, args[0], *sample, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "prune":
		fs := newFlagSet("prune")
		cfgPath := fs.String("config", "", "")
		keepLast := fs.Int("keep-last", -1, "")
		keepMonthly := fs.Int("keep-monthly", 0, "")
		dry := fs.Bool("dry-run", false, "")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedPruneFlags)); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		var removeIDs []string
		for _, a := range fs.Args() {
			s := strings.TrimSpace(a)
			if s == "" {
				fmt.Fprintln(os.Stderr, "prune: empty snapshot id")
				return 1
			}
			removeIDs = append(removeIDs, s)
		}
		if len(removeIDs) > 0 {
			if *keepLast != -1 || *keepMonthly != 0 {
				fmt.Fprintln(os.Stderr, "prune: do not use --keep-last or --keep-monthly with explicit snapshot ids")
				return 1
			}
		} else if *keepLast < 1 {
			fmt.Fprintln(os.Stderr, "prune: --keep-last must be >= 1 (or pass snapshot id(s) to remove)")
			return 1
		}
		cp := *cfgPath
		if cp == "" {
			cp = defaultConfigPath()
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
		if err := pruneRunHook(ctx, st, cfg.HostID, removeIDs, *keepLast, *keepMonthly, *dry, log); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0

	case "export-recovery-kit":
		fs := newFlagSet("export-recovery-kit")
		outDir := fs.String("output", "", "directory to write FORMAT.md and hints")
		cfgPath := fs.String("config", "", "optional config for bucket hints and public recipients")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedExportRecoveryKitFlags)); err != nil {
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
		if err := exportRecoveryKitHook(*outDir, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "wrote recovery kit to %s\n", *outDir)
		return 0

	case "status":
		fs := newFlagSet("status")
		cfgPath := fs.String("config", "", "")
		host := fs.String("host", "", "")
		showCost := fs.Bool("show-cost", false, "")
		v := fs.Bool("v", false, "")
		vv := fs.Bool("vv", false, "")
		if err := fs.Parse(reorderFlagsBeforePositionals(argv[2:], commandValuedStatusFlags)); err != nil {
			return 1
		}
		log := loggerFromVerbosity(*v, *vv)
		cp := *cfgPath
		if cp == "" {
			cp = defaultConfigPath()
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
		if err := statusRunHook(ctx, st, *host, *showCost, log); err != nil {
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
  prune (--keep-last N [--keep-monthly M] | <snapshot-id>...) [--dry-run] [--config PATH] [-v]
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
	awscfg, err := awsLoadForStore(ctx, cfg.S3.Region)
	if err != nil {
		return nil, err
	}
	return newS3Store(ctx, awscfg, cfg.S3.Bucket)
}
