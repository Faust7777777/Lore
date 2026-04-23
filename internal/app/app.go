package app

import "strings"

const (
	defaultName      = "lore"
	defaultVersion   = "dev"
	defaultState     = "bootstrap"
	defaultViewMode  = "text"
	defaultVaultPath = "(unconfigured)"
	defaultProfile   = "default"
	defaultTransport = "local"
)

type Config struct {
	Version   string
	VaultPath string
	Profile   string
	State     string
}

type Status struct {
	Name          string
	Version       string
	State         string
	ViewMode      string
	VaultPath     string
	ActiveProfile string
	Transport     string
}

type Application struct {
	status Status
}

func New(cfg Config) *Application {
	return &Application{
		status: Status{
			Name:          defaultName,
			Version:       withFallback(cfg.Version, defaultVersion),
			State:         withFallback(cfg.State, defaultState),
			ViewMode:      defaultViewMode,
			VaultPath:     withFallback(cfg.VaultPath, defaultVaultPath),
			ActiveProfile: withFallback(cfg.Profile, defaultProfile),
			Transport:     defaultTransport,
		},
	}
}

func (a *Application) Status() Status {
	return a.status
}

func withFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}

	return value
}
