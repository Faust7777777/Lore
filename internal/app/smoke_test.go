package app

import (
	"testing"
	"time"
)

func TestRuntimeSmokeP0(t *testing.T) {
	runtime, err := openRuntimeWithFakeProcessSinkSummarizer(t, t.TempDir())
	if err != nil {
		t.Fatalf("OpenRuntime() error = %v", err)
	}

	result, err := runtime.SmokeP0(time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SmokeP0() error = %v", err)
	}
	if !result.OK() {
		t.Fatalf("SmokeP0() checks failed: %+v", result.Checks)
	}
	if !result.Managed.Ready {
		t.Fatal("Managed.Ready = false, want true")
	}
	if result.Draft.ID == "" || result.Checkpoint.Path == "" || result.Report.Path == "" {
		t.Fatalf("result = %+v, want populated draft/checkpoint/report", result)
	}
}
