package mcp

import "testing"

// TestValidateProcessAuth covers the constant-time client-key check
// (review-v1 P1-15). It exercises the compare logic; the constant-time
// property itself is guaranteed by crypto/subtle.
func TestValidateProcessAuth(t *testing.T) {
	// Isolate every env var the check reads.
	for _, k := range []string{
		"LORE_MCP_API_KEY", "OBSIDIAN_HARNESS_MCP_API_KEY",
		"LORE_CLIENT_KEY", "OBSIDIAN_HARNESS_CLIENT_KEY",
	} {
		t.Setenv(k, "")
	}

	// No key configured -> auth not required.
	if err := validateProcessAuth(); err != nil {
		t.Fatalf("no key configured should pass, got %v", err)
	}

	t.Setenv("LORE_MCP_API_KEY", "s3cret-key")

	// Matching client key -> pass.
	t.Setenv("LORE_CLIENT_KEY", "s3cret-key")
	if err := validateProcessAuth(); err != nil {
		t.Fatalf("matching key should pass, got %v", err)
	}

	// Mismatched -> fail.
	t.Setenv("LORE_CLIENT_KEY", "wrong-key")
	if err := validateProcessAuth(); err == nil {
		t.Fatal("mismatched key should fail")
	}

	// Different length (ConstantTimeCompare returns 0 for unequal length) -> fail.
	t.Setenv("LORE_CLIENT_KEY", "s3cret-key-longer")
	if err := validateProcessAuth(); err == nil {
		t.Fatal("different-length key should fail")
	}

	// Empty client key against a configured key -> fail.
	t.Setenv("LORE_CLIENT_KEY", "")
	if err := validateProcessAuth(); err == nil {
		t.Fatal("empty client key against a configured key should fail")
	}
}
