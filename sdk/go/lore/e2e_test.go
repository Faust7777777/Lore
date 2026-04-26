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

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, filepath.Join(repoRoot, ".tools", "go", "bin", goExeName()), "build", "-o", binPath, "./cmd/lore")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lore: %v\n%s", err, string(output))
	}

	bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootstrapCancel()
	bootstrap := exec.CommandContext(bootstrapCtx, binPath, "bootstrap", workDir)
	if output, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap lore workdir: %v\n%s", err, string(output))
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := Start(startCtx, Options{Command: binPath, WorkDir: workDir})
	startCancel()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer client.Close()

	callCtx, callCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer callCancel()
	if err := client.Ping(callCtx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	status, err := client.ManagedStatus(callCtx)
	if err != nil {
		t.Fatalf("ManagedStatus() error = %v", err)
	}
	if !status.Ready {
		t.Fatalf("ManagedStatus().Ready = false, want true")
	}
	resolved, err := client.VaultResolve(callCtx, VaultResolveRequest{Query: "progress", Limit: 5})
	if err != nil {
		t.Fatalf("VaultResolve() error = %v", err)
	}
	if resolved.Status == "" {
		t.Fatalf("VaultResolve().Status is empty: %+v", resolved)
	}
	pack, err := client.ContextPack(callCtx, ContextPackRequest{Task: "progress", Limit: 3})
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
