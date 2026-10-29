package masking

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSQLTransforms(t *testing.T) {
	tests := []struct {
		col       string
		generator string
		contains  string
	}{
		{"email", "faker.email", "@masked.dev"},
		{"full_name", "faker.name", "Masked User #"},
		{"phone", "faker.phone", "+1-555-"},
		{"company", "faker.company", "Organization "},
		{"notes", "null", "NULL"},
		{"code", "constant:SECRET_VAL", "'SECRET_VAL'"},
		{"token", "truncate", "''"},
	}

	for _, tt := range tests {
		expr := GenerateSQLTransforms(tt.col, tt.generator)
		if expr == "" {
			t.Errorf("expected non-empty expression for %s", tt.generator)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "mask.yaml")
	yamlContent := `
rules:
  users:
    email: "faker.email"
    full_name: "faker.name"
  audit_logs:
    ip_address: "constant:127.0.0.1"
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("failed loading config: %v", err)
	}

	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(cfg.Rules))
	}
	if cfg.Rules["users"]["email"] != "faker.email" {
		t.Errorf("expected faker.email, got %s", cfg.Rules["users"]["email"])
	}
	if cfg.Rules["audit_logs"]["ip_address"] != "constant:127.0.0.1" {
		t.Errorf("expected constant:127.0.0.1, got %s", cfg.Rules["audit_logs"]["ip_address"])
	}
}
