package cli

import (
	"fmt"
	"strings"

	"obsidian-harness/internal/config"
)

// renderLLMConfigDiagnostics formats safe model-provider diagnostics for status
// output. It deliberately omits API key values; only api_key_env/api_key_ref
// names are shown so operators can see the source without leaking secrets.
func renderLLMConfigDiagnostics(diagnostics []config.LLMDiagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\nModel Config\n")
	builder.WriteString("------------\n")
	for _, diag := range diagnostics {
		purpose := strings.TrimSpace(diag.Purpose)
		if purpose == "" {
			purpose = "unknown"
		}
		if diag.Err != nil {
			fmt.Fprintf(&builder, "%s: error source=%s", purpose, diag.Source)
			if diag.Profile != "" {
				fmt.Fprintf(&builder, " profile=%s", diag.Profile)
			}
			if diag.KeyStatus != "" {
				fmt.Fprintf(&builder, " key=%s", diag.KeyStatus)
			}
			fmt.Fprintf(&builder, " err=%s\n", safeDiagnosticError(diag.Err))
			continue
		}
		if !diag.Enabled {
			fmt.Fprintf(&builder, "%s: disabled source=%s\n", purpose, diag.Source)
			continue
		}
		fmt.Fprintf(&builder, "%s: provider=%s model=%s source=%s", purpose, diag.Provider, diag.Model, diag.Source)
		if diag.Profile != "" {
			fmt.Fprintf(&builder, " profile=%s", diag.Profile)
		}
		if diag.BaseURL != "" {
			fmt.Fprintf(&builder, " base_url=%s", diag.BaseURL)
		}
		if diag.APIKeyEnv != "" {
			fmt.Fprintf(&builder, " api_key_env=%s", diag.APIKeyEnv)
		}
		if diag.APIKeyRef != "" {
			fmt.Fprintf(&builder, " api_key_ref=%s", diag.APIKeyRef)
		}
		if diag.KeyStatus != "" {
			fmt.Fprintf(&builder, " key=%s", diag.KeyStatus)
		}
		if diag.Timeout > 0 {
			fmt.Fprintf(&builder, " timeout=%s", diag.Timeout)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func safeDiagnosticError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "authorization") || strings.Contains(lower, "bearer ") {
		return "[redacted credential-bearing error]"
	}
	return msg
}
