package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerfileFor_AllLanguages(t *testing.T) {
	for _, lang := range []string{"cpp", "rust", "go", "python"} {
		df := dockerfileFor(lang)
		if df == "" {
			t.Fatalf("empty Dockerfile for %s", lang)
		}
		if !strings.Contains(df, "EXPOSE 8080") {
			t.Errorf("%s Dockerfile missing EXPOSE 8080", lang)
		}
		if !strings.Contains(df, "HEALTHCHECK") {
			t.Errorf("%s Dockerfile missing HEALTHCHECK", lang)
		}
	}
	if dockerfileFor("java") != "" {
		t.Error("expected empty Dockerfile for unsupported language")
	}
}

func TestUnzip_RejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../../etc/passwd")
	_, _ = w.Write([]byte("malicious"))
	_ = zw.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := unzip(zipPath, dest); err == nil {
		t.Fatal("expected zip-slip to be rejected")
	}
}

func TestUnzip_ExtractsValid(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("main.cpp")
	_, _ = w.Write([]byte("int main(){}"))
	_ = zw.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := unzip(zipPath, dest); err != nil {
		t.Fatalf("unzip failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "main.cpp")); err != nil {
		t.Fatalf("expected main.cpp extracted: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 3); got != "hel" {
		t.Errorf("got %q", got)
	}
	if got := truncate("hi", 5); got != "hi" {
		t.Errorf("got %q", got)
	}
}
