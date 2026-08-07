package config

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

// setEnv 临时设置环境变量，测试结束自动恢复。
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	orig := map[string]string{}
	for k, v := range kv {
		orig[k] = os.Getenv(k)
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		for k, v := range orig {
			os.Setenv(k, v)
		}
		ResetForTest()
	})
}

func TestGetDSNRestricted(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{
		"PG_HOST": "db.example.com", "PG_PORT": "5433", "PG_USER": "alice",
		"PG_PASSWORD": "secret", "PG_DATABASE": "app", "PG_SCHEMA": "public",
		"PG_ACCESS_MODE": "restricted", "PG_STATEMENT_TIMEOUT": "5000",
	})
	cfg := LoadConfig()

	u, err := url.Parse(cfg.GetDSN())
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	q := u.Query()
	if q.Get("default_transaction_read_only") != "on" {
		t.Errorf("restricted DSN missing default_transaction_read_only=on")
	}
	if q.Get("statement_timeout") != "5000" {
		t.Errorf("statement_timeout = %q, want 5000", q.Get("statement_timeout"))
	}
	if q.Get("search_path") != "public,pg_temp" {
		t.Errorf("search_path = %q, want public,pg_temp", q.Get("search_path"))
	}
	if q.Get("application_name") != "pg-mcp" {
		t.Errorf("application_name = %q", q.Get("application_name"))
	}
	if q.Get("sslmode") != "prefer" {
		t.Errorf("sslmode = %q", q.Get("sslmode"))
	}
	if !strings.HasPrefix(cfg.GetDSN(), "postgresql://alice:secret@db.example.com:5433/app") {
		t.Errorf("dsn = %q", cfg.GetDSN())
	}
	if !cfg.IsRestricted() {
		t.Error("IsRestricted = false")
	}
	if !cfg.IsValid() {
		t.Error("IsValid = false")
	}
}

func TestGetDSNUnrestricted(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{
		"PG_HOST": "localhost", "PG_PORT": "5432", "PG_USER": "postgres",
		"PG_PASSWORD": "secret", "PG_DATABASE": "postgres", "PG_ACCESS_MODE": "unrestricted",
	})
	cfg := LoadConfig()
	u, _ := url.Parse(cfg.GetDSN())
	if v := u.Query().Get("default_transaction_read_only"); v != "" {
		t.Errorf("unrestricted DSN has read_only=%q", v)
	}
	if cfg.IsRestricted() {
		t.Error("IsRestricted = true")
	}
}

func TestGetDSNRawOverride(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{"PG_DSN": "postgresql://u:p@h:5432/db"})
	cfg := LoadConfig()
	if cfg.GetDSN() != "postgresql://u:p@h:5432/db" {
		t.Errorf("raw DSN override failed: %q", cfg.GetDSN())
	}
}

func TestInvalidAccessModeFallsBackToRestricted(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{"PG_ACCESS_MODE": "bogus", "PG_PASSWORD": "x"})
	cfg := LoadConfig()
	if !cfg.IsRestricted() {
		t.Error("invalid access mode should fall back to restricted")
	}
}

func TestMaskedDSNHidesPassword(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{
		"PG_HOST": "localhost", "PG_PORT": "5432", "PG_USER": "postgres",
		"PG_PASSWORD": "super-secret", "PG_DATABASE": "postgres",
	})
	m := LoadConfig().MaskedDSN()
	if strings.Contains(m, "super-secret") {
		t.Errorf("MaskedDSN leaks password: %q", m)
	}
	if !strings.Contains(m, "***") {
		t.Errorf("MaskedDSN should contain ***: %q", m)
	}
}

func TestConfigInvalidWithoutPassword(t *testing.T) {
	ResetForTest()
	setEnv(t, map[string]string{"PG_PASSWORD": "", "PG_DSN": ""})
	cfg := LoadConfig()
	if cfg.IsValid() {
		t.Error("IsValid = true without password")
	}
}
