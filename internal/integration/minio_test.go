//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/banerjs/cairn/internal/ageutil"
	"github.com/banerjs/cairn/internal/backup"
	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/restore"
	"github.com/banerjs/cairn/internal/s3store"

	"filippo.io/age"
)

func TestBackupRestoreMinIO(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(120 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	defer func() { _ = c.Terminate(ctx) }()

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := c.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec := id.Recipient()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	idPath := filepath.Join(tmp, "id.age")
	if err := os.WriteFile(idPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, "config.toml")
	cfgBody := fmt.Sprintf(`
host_id = "int-host"
cleanup_grace = "1ns"

[s3]
bucket = "cairn-int"
region = "us-east-1"
storage_class = "STANDARD"

[encryption]
recipients = [%q]
identity_file = %q

[backup]
source_roots = [%q]
parallelism = 2
`, rec.String(), idPath, src)
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := appcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	awsCfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", ""),
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(cfg.S3.Bucket)})
	if err != nil {
		t.Fatal(err)
	}

	st := s3store.NewWithClient(client, cfg.S3.Bucket)
	recips, err := ageutil.ParsePQRecipients(cfg.Encryption.Recipients)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := backup.Run(ctx, cfg, st, recips, "", 2, log); err != nil {
		t.Fatal(err)
	}

	ids, err := ageutil.LoadIdentities(idPath)
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")
	snapID := lastSnapshotID(t, ctx, st, cfg.HostID)
	if snapID == "" {
		t.Fatal("no snapshot")
	}
	if err := restore.Run(ctx, cfg, st, ids, snapID, outDir, 2, log); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("got %q", got)
	}
}

func lastSnapshotID(t *testing.T, ctx context.Context, st *s3store.Store, host string) string {
	t.Helper()
	prefix := paths.SnapshotsListPrefix(host)
	objs, err := st.ListPrefix(ctx, prefix)
	if err != nil {
		t.Fatal(err)
	}
	var best string
	for _, o := range objs {
		if !strings.HasSuffix(o.Key, "manifest.age") {
			continue
		}
		sid := paths.SnapshotIDFromKey(o.Key)
		if sid == "" {
			continue
		}
		if sid > best {
			best = sid
		}
	}
	return best
}
