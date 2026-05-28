package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type EditableConfig struct {
	WorkDir         string
	WorkspacePath   string
	WorkspaceRaw    map[string]json.RawMessage
	WorkspaceExists bool
}

type WorkspaceConfigPatch struct {
	LLM *LLMConfig
}

func LoadEditableConfig(workDir string) (EditableConfig, error) {
	path, err := workspaceConfigPath(workDir)
	if err != nil {
		return EditableConfig{}, err
	}
	raw, exists, err := readWorkspaceRaw(path)
	if err != nil {
		return EditableConfig{}, err
	}
	return EditableConfig{
		WorkDir:         workDir,
		WorkspacePath:   path,
		WorkspaceRaw:    raw,
		WorkspaceExists: exists,
	}, nil
}

func SaveWorkspaceConfig(workDir string, patch WorkspaceConfigPatch) error {
	editable, err := LoadEditableConfig(workDir)
	if err != nil {
		return err
	}
	raw := cloneRawObject(editable.WorkspaceRaw)
	if patch.LLM != nil {
		data, err := json.MarshalIndent(patch.LLM, "", "  ")
		if err != nil {
			return fmt.Errorf("config: marshal llm workspace patch: %w", err)
		}
		raw["llm"] = data
	}
	return writeWorkspaceRaw(editable.WorkspacePath, raw)
}

func UpsertLLMProfile(workDir string, profileName string, profile LLMProfileConfig) error {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return fmt.Errorf("config: llm profile name must not be empty")
	}
	profile = normalizeLLMProfileConfig(profile)
	if err := validateWorkspaceLLMProfile(profileName, profile); err != nil {
		return err
	}
	editable, err := LoadEditableConfig(workDir)
	if err != nil {
		return err
	}
	llm, err := editableLLMConfig(editable)
	if err != nil {
		return err
	}
	if llm.Profiles == nil {
		llm.Profiles = make(map[string]LLMProfileConfig)
	}
	llm.Profiles[profileName] = profile
	return SaveWorkspaceConfig(workDir, WorkspaceConfigPatch{LLM: &llm})
}

func SetActiveLLMProfile(workDir string, profileName string) error {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return fmt.Errorf("config: active llm profile name must not be empty")
	}
	editable, err := LoadEditableConfig(workDir)
	if err != nil {
		return err
	}
	llm, err := editableLLMConfig(editable)
	if err != nil {
		return err
	}
	if _, ok := llm.Profiles[profileName]; !ok {
		return fmt.Errorf("config: llm profile %q does not exist", profileName)
	}
	llm.ActiveProfile = profileName
	return SaveWorkspaceConfig(workDir, WorkspaceConfigPatch{LLM: &llm})
}

func editableLLMConfig(editable EditableConfig) (LLMConfig, error) {
	raw, ok := editable.WorkspaceRaw["llm"]
	if !ok || len(strings.TrimSpace(string(raw))) == 0 {
		return LLMConfig{}, nil
	}
	var llm LLMConfig
	if err := json.Unmarshal(raw, &llm); err != nil {
		return LLMConfig{}, fmt.Errorf("config: parse workspace llm at %s: %w", editable.WorkspacePath, err)
	}
	return llm, nil
}

func normalizeLLMProfileConfig(profile LLMProfileConfig) LLMProfileConfig {
	profile.Provider = strings.TrimSpace(profile.Provider)
	if profile.Provider == "" {
		profile.Provider = "openai-compatible"
	}
	profile.BaseURL = strings.TrimSpace(profile.BaseURL)
	profile.Model = strings.TrimSpace(profile.Model)
	profile.APIKeyEnv = strings.TrimSpace(profile.APIKeyEnv)
	profile.APIKeyRef = strings.TrimSpace(profile.APIKeyRef)
	return profile
}

func validateWorkspaceLLMProfile(profileName string, profile LLMProfileConfig) error {
	if strings.TrimSpace(profile.BaseURL) == "" {
		return fmt.Errorf("config: llm profile %q base_url must not be empty", profileName)
	}
	if strings.TrimSpace(profile.Model) == "" {
		return fmt.Errorf("config: llm profile %q model must not be empty", profileName)
	}
	if strings.TrimSpace(profile.APIKeyEnv) == "" {
		return fmt.Errorf("config: llm profile %q api_key_env must be set; API keys must stay outside repo, vault, and sessionlog", profileName)
	}
	return nil
}

func workspaceConfigPath(workDir string) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return "", fmt.Errorf("config: workDir must not be empty")
	}
	return filepath.Join(workDir, ".lore", "config.json"), nil
}

func readWorkspaceRaw(path string) (map[string]json.RawMessage, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, false, nil
		}
		return nil, false, fmt.Errorf("config: read workspace config at %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]json.RawMessage{}, true, nil
	}
	var probe any
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, true, fmt.Errorf("config: parse workspace config at %s: %w", path, err)
	}
	if _, ok := probe.(map[string]any); !ok {
		return nil, true, fmt.Errorf("config: workspace config at %s must be a JSON object", path)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, true, fmt.Errorf("config: parse workspace config at %s: %w", path, err)
	}
	return cloneRawObject(raw), true, nil
}

func writeWorkspaceRaw(path string, raw map[string]json.RawMessage) error {
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal workspace config: %w", err)
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o600)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: create workspace config dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp workspace config: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: write temp workspace config: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: chmod temp workspace config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close temp workspace config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config: replace workspace config at %s: %w", path, err)
	}
	cleanup = false
	return nil
}

func cloneRawObject(raw map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
