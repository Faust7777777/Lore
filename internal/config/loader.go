package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LayerSource identifies where a configuration layer comes from.
type LayerSource string

const (
	LayerDefault    LayerSource = "default"
	LayerUserGlobal LayerSource = "user_global"
	LayerWorkspace  LayerSource = "workspace"
)

// LayerStatus describes whether a particular layer was applied.
type LayerStatus string

const (
	LayerStatusLoaded  LayerStatus = "loaded"
	LayerStatusMissing LayerStatus = "missing"
	LayerStatusError   LayerStatus = "error"
)

// LoadDiagnostic records the outcome of one configuration layer.
type LoadDiagnostic struct {
	Source LayerSource
	Path   string
	Status LayerStatus
	Err    error
}

// LoadOptions controls layer file paths. Empty fields use the defaults derived
// from the OS user home directory and the workspace directory.
type LoadOptions struct {
	// UserGlobalPath, if non-empty, replaces the default ~/.lore/config.json
	// path. Pass an explicit path to point Load at a specific override file in
	// tests or non-standard environments.
	UserGlobalPath string

	// WorkspacePath, if non-empty, replaces the default
	// <workDir>/.lore/config.json path. Empty workspace path with empty
	// workDir disables the workspace layer entirely.
	WorkspacePath string
}

// Load builds a Config by merging the following layers in order of increasing
// precedence:
//
//  1. Defaults from Default(workDir).
//  2. User-global override file: ~/.lore/config.json (skipped silently if the
//     home directory cannot be resolved or the file does not exist).
//  3. Workspace override file: <workDir>/.lore/config.json (skipped silently
//     if workDir is empty or the file does not exist).
//
// Each layer's JSON file may set any subset of fields; fields it does not
// mention keep the lower layer's value. Malformed JSON or filesystem errors
// other than "missing file" are returned with a partial Config built from the
// layers that succeeded so far. The returned diagnostics list every layer
// Load considered, including missing and errored ones, in load order.
//
// Note: time.Duration fields in layer files must be expressed as nanoseconds
// (for example 500000000 for 500ms). String forms such as "500ms" are not
// yet supported and would surface as a JSON parse error.
//
// Test isolation: Load reads from os.UserHomeDir() by default. Tests that
// must avoid picking up a real user file should pass an explicit
// LoadOptions.UserGlobalPath pointing to an absent or controlled location.
func Load(workDir string) (Config, []LoadDiagnostic, error) {
	return LoadWithOptions(workDir, LoadOptions{})
}

// LoadWithOptions is like Load but uses the override paths from opts.
func LoadWithOptions(workDir string, opts LoadOptions) (Config, []LoadDiagnostic, error) {
	diagnostics := make([]LoadDiagnostic, 0, 3)
	cfg := Default(workDir)
	diagnostics = append(diagnostics, LoadDiagnostic{Source: LayerDefault, Status: LayerStatusLoaded})

	userPath := strings.TrimSpace(opts.UserGlobalPath)
	if userPath == "" {
		userPath = defaultUserGlobalConfigPath()
	}
	if userPath != "" {
		diag, err := applyLayerFile(&cfg, LayerUserGlobal, userPath)
		diagnostics = append(diagnostics, diag)
		if err != nil {
			return cfg, diagnostics, err
		}
	}

	workspacePath := strings.TrimSpace(opts.WorkspacePath)
	if workspacePath == "" && strings.TrimSpace(workDir) != "" {
		workspacePath = filepath.Join(workDir, ".lore", "config.json")
	}
	if workspacePath != "" {
		diag, err := applyLayerFile(&cfg, LayerWorkspace, workspacePath)
		diagnostics = append(diagnostics, diag)
		if err != nil {
			return cfg, diagnostics, err
		}
	}

	return cfg, diagnostics, nil
}

func defaultUserGlobalConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return ""
	}
	return filepath.Join(homeDir, ".lore", "config.json")
}

func applyLayerFile(cfg *Config, source LayerSource, path string) (LoadDiagnostic, error) {
	diag := LoadDiagnostic{Source: source, Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			diag.Status = LayerStatusMissing
			return diag, nil
		}
		diag.Status = LayerStatusError
		diag.Err = err
		return diag, fmt.Errorf("config: read %s layer at %s: %w", source, path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		diag.Status = LayerStatusMissing
		return diag, nil
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		diag.Status = LayerStatusError
		diag.Err = err
		return diag, fmt.Errorf("config: parse %s layer at %s: %w", source, path, err)
	}
	diag.Status = LayerStatusLoaded
	return diag, nil
}
