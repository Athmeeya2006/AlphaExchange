package security

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
)

// ImageScanner runs Trivy against built contestant images. If Trivy is not
// installed it degrades to a no-op (logged once) so local dev still works.
type ImageScanner struct {
	logger    *slog.Logger
	available bool
}

// NewImageScanner constructs the scanner, detecting whether trivy is present.
func NewImageScanner(logger *slog.Logger) *ImageScanner {
	_, err := exec.LookPath("trivy")
	if err != nil {
		logger.Warn("trivy not found; image scanning disabled")
	}
	return &ImageScanner{logger: logger, available: err == nil}
}

// ScanResult reports the worst severity found.
type ScanResult struct {
	HasCritical bool
	HasHigh     bool
	Output      string
}

// Scan runs `trivy image --severity HIGH,CRITICAL`. CRITICAL findings should
// block the container from starting.
func (s *ImageScanner) Scan(ctx context.Context, imageName string) (ScanResult, error) {
	if !s.available {
		return ScanResult{}, nil
	}
	cmd := exec.CommandContext(ctx, "trivy", "image",
		"--quiet", "--severity", "HIGH,CRITICAL", "--no-progress", imageName)
	out, err := cmd.CombinedOutput()
	res := ScanResult{Output: string(out)}
	// trivy exit code is 0 unless --exit-code is set; we parse the text instead.
	res.HasCritical = strings.Contains(res.Output, "CRITICAL")
	res.HasHigh = strings.Contains(res.Output, "HIGH")
	if err != nil {
		// A non-zero exit can still carry useful output; surface it but don't fail hard.
		s.logger.Warn("trivy scan returned error", "error", err, "image", imageName)
	}
	return res, nil
}
