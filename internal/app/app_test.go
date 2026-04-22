package app

import "testing"

func TestNewProvidesDefaults(t *testing.T) {
	application := New(Config{})

	status := application.Status()
	if status.Name != "obsidian-harness" {
		t.Fatalf("expected default name, got %q", status.Name)
	}
	if status.Version != "dev" {
		t.Fatalf("expected default version dev, got %q", status.Version)
	}
	if status.State != "bootstrap" {
		t.Fatalf("expected default state bootstrap, got %q", status.State)
	}
	if status.ViewMode != "text" {
		t.Fatalf("expected default view mode text, got %q", status.ViewMode)
	}
	if status.VaultPath != "(unconfigured)" {
		t.Fatalf("expected default vault path, got %q", status.VaultPath)
	}
}

func TestNewHonorsProvidedConfig(t *testing.T) {
	application := New(Config{
		Version:   "1.2.3",
		VaultPath: `C:\Users\me\Documents\vault`,
		Profile:   "study",
	})

	status := application.Status()
	if status.Version != "1.2.3" {
		t.Fatalf("expected configured version, got %q", status.Version)
	}
	if status.VaultPath != `C:\Users\me\Documents\vault` {
		t.Fatalf("expected configured vault path, got %q", status.VaultPath)
	}
	if status.ActiveProfile != "study" {
		t.Fatalf("expected configured profile, got %q", status.ActiveProfile)
	}
}
