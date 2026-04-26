package lore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSDKEndToEndWithLoreMCP(t *testing.T) {
	if os.Getenv("LORE_SDK_E2E") != "1" {
		t.Skip("set LORE_SDK_E2E=1 to run SDK end-to-end test")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	workDir := t.TempDir()
	binPath := filepath.Join(t.TempDir(), "lore")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	build := exec.Command(filepath.Join(repoRoot, ".tools", "go", "bin", goExeName()), "build", "-o", binPath, "./cmd/lore")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lore: %v\n%s", err, string(output))
	}
	bootstrap := exec.Command(binPath, "bootstrap", workDir)
	if output, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap lore workdir: %v\n%s", err, string(output))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Start(ctx, Options{Command: binPath, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	status, err := client.ManagedStatus(ctx)
	if err != nil {
		t.Fatalf("ManagedStatus() error = %v", err)
	}
	if !status.Ready {
		t.Fatalf("ManagedStatus().Ready = false, want true")
	}
	resolved, err := client.VaultResolve(ctx, VaultResolveRequest{Query: "progress", Limit: 5})
	if err != nil {
		t.Fatalf("VaultResolve() error = %v", err)
	}
	if resolved.Status == "" {
		t.Fatalf("VaultResolve().Status is empty: %+v", resolved)
	}
	pack, err := client.ContextPack(ctx, ContextPackRequest{Task: "progress", Limit: 3})
	if err != nil {
		t.Fatalf("ContextPack() error = %v", err)
	}
	if pack.ProgressDoc == nil {
		t.Fatalf("ContextPack().ProgressDoc = nil")
	}
}

func goExeName() string {
	if runtime.GOOS == "windows" {
		return "go.exe"
	}
	return "go"
}
