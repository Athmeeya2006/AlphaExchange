package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"

	"github.com/trade-eval/build-worker/security"
)

// Worker runs the full build pipeline for a single submission.
type Worker struct {
	cfg      Config
	logger   *slog.Logger
	docker   *client.Client
	minio    *minio.Client
	pool     *pgxpool.Pool
	redis    *redis.Client
	producer *Producer
	cm       *ContainerManager
	scanner  *security.ImageScanner
	monitor  *security.ResourceMonitor
}

// ProcessBuild executes download -> build -> sandbox -> health -> ready.
func (wk *Worker) ProcessBuild(ctx context.Context, job BuildJob) {
	log := wk.logger.With("submission_id", job.SubmissionID, "language", job.Language)
	log.Info("processing build")

	// STEP 1 - building.
	wk.setStatus(ctx, job.SubmissionID, "building", "")

	// STEP 2 - temp dir.
	dir := filepath.Join(wk.cfg.WorkDir, job.SubmissionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		wk.fail(ctx, job, "could not create workdir: "+err.Error())
		return
	}
	defer os.RemoveAll(dir)

	// STEP 3 - download + unzip.
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		wk.fail(ctx, job, err.Error())
		return
	}
	if err := wk.downloadAndExtract(ctx, job, srcDir); err != nil {
		wk.fail(ctx, job, "download/extract failed: "+err.Error())
		return
	}

	// STEP 4 - build (600s budget - includes pulling base images on first run).
	buildCtx, cancel := context.WithTimeout(ctx, 600*time.Second)
	defer cancel()
	imageName, buildLogs, err := buildContainer(buildCtx, wk.docker, job.SubmissionID, job.Language, srcDir)
	if err != nil {
		// STEP 5 - build failure.
		wk.fail(ctx, job, truncate(buildLogs, 10*1024))
		return
	}

	// STEP 5b - image vulnerability scan; CRITICAL findings block the run.
	if wk.scanner != nil {
		if res, _ := wk.scanner.Scan(buildCtx, imageName); res.HasCritical {
			wk.fail(ctx, job, "security_scan_failed: CRITICAL vulnerabilities found\n"+truncate(res.Output, 4096))
			return
		}
	}

	// STEP 6 - launch sandbox.
	containerID, ip, port, err := launchSandbox(ctx, wk.docker, wk.cfg, imageName, job.SubmissionID)
	if err != nil {
		wk.fail(ctx, job, "sandbox launch failed: "+err.Error())
		return
	}

	// STEP 7 - health probe (30s, every 2s). Use host port for probing.
	if !wk.waitHealthy(ctx, ip, port, 30*time.Second) {
		_ = wk.cm.StopContainer(ctx, job.SubmissionID)
		wk.fail(ctx, job, "container did not become healthy")
		return
	}

	// STEP 8 - ready.
	wk.setContainerInfo(ctx, job.SubmissionID, ip, port, containerID)
	wk.setStatus(ctx, job.SubmissionID, "ready", "")
	if wk.redis != nil {
		wk.redis.HSet(ctx, "container:"+job.SubmissionID, "ip", ip, "port", port, "status", "ready")
	}
	wk.cm.Track(job.SubmissionID, ContainerInfo{
		ContainerID:  containerID,
		ImageName:    imageName,
		IP:           ip,
		Port:         port,
		ContestantID: job.ContestantID,
		CreatedAt:    time.Now(),
	})

	// STEP 9 - publish CONTAINER_READY + start resource monitoring.
	wk.publishReady(ctx, job, ip, port)
	if wk.monitor != nil {
		softMem := uint64(float64(wk.cfg.SandboxMemoryMB) * 1024 * 1024 * 0.8)
		go wk.monitor.Watch(context.Background(), containerID, job.SubmissionID, softMem, func(reason string) {
			wk.logger.Warn("resource alert", "submission_id", job.SubmissionID, "reason", reason)
		})
	}
	log.Info("build ready", "ip", ip, "port", port)
}

func (wk *Worker) downloadAndExtract(ctx context.Context, job BuildJob, srcDir string) error {
	obj, err := wk.minio.GetObject(ctx, wk.cfg.MinIOBucket, job.S3Key, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer obj.Close()

	zipPath := filepath.Join(filepath.Dir(srcDir), "source.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, obj); err != nil {
		out.Close()
		return err
	}
	out.Close()

	// A bare .cpp upload is wrapped as src/main.cpp; a .zip is extracted.
	if strings.HasSuffix(strings.ToLower(job.S3Key), ".cpp") {
		data, err := os.ReadFile(zipPath)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(srcDir, "main.cpp"), data, 0o644)
	}
	return unzip(zipPath, srcDir)
}

func unzip(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := checkZipBomb(&zr.Reader); err != nil {
		return err
	}
	for _, f := range zr.File {
		// zip-slip guard.
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 100*1024*1024))
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (wk *Worker) waitHealthy(ctx context.Context, host string, port int, timeout time.Duration) bool {
	if port == 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://%s:%d/health", host, port)
	cl := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := cl.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return false
}

func (wk *Worker) setStatus(ctx context.Context, id, status, errorLog string) {
	const q = `UPDATE submissions SET status = $2, error_log = NULLIF($3,'') WHERE id = $1`
	if _, err := wk.pool.Exec(ctx, q, id, status, errorLog); err != nil {
		wk.logger.Error("update status failed", "error", err, "submission_id", id)
	}
}

func (wk *Worker) setContainerInfo(ctx context.Context, id, ip string, port int, containerID string) {
	const q = `UPDATE submissions SET container_ip = $2, container_port = $3, container_id = $4 WHERE id = $1`
	if _, err := wk.pool.Exec(ctx, q, id, ip, port, containerID); err != nil {
		wk.logger.Error("update container info failed", "error", err)
	}
}

func (wk *Worker) fail(ctx context.Context, job BuildJob, reason string) {
	wk.logger.Warn("build failed", "submission_id", job.SubmissionID, "reason", truncate(reason, 256))
	wk.setStatus(ctx, job.SubmissionID, "failed", reason)
}

func (wk *Worker) publishReady(ctx context.Context, job BuildJob, ip string, port int) {
	if wk.producer == nil {
		return
	}
	evt := ContainerReadyEvent{
		Event:         "CONTAINER_READY",
		SubmissionID:  job.SubmissionID,
		ContestantID:  job.ContestantID,
		ContainerIP:   ip,
		ContainerPort: port,
		Status:        "ready",
	}
	if b, err := json.Marshal(evt); err == nil {
		if err := wk.producer.Produce(ctx, wk.cfg.OrchEventsTopic, []byte(job.ContestantID), b); err != nil {
			wk.logger.Warn("publish CONTAINER_READY failed", "error", err)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
