package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config represents runtime settings for BranchDB.
type Config struct {
	ProxyHost        string `json:"proxy_host"`
	ProxyPort        int    `json:"proxy_port"`
	UpstreamHost     string `json:"upstream_host"`
	UpstreamPort     int    `json:"upstream_port"`
	UpstreamUser     string `json:"upstream_user"`
	UpstreamPassword string `json:"upstream_password"`
	UpstreamSSLMode  string `json:"upstream_ssl_mode"`
	BaseDatabase     string `json:"base_database"`
	BranchPrefix     string `json:"branch_prefix"`
	Namespace        string `json:"namespace"`
	SeedFile         string `json:"seed_file"`
	RepoPath         string `json:"repo_path"`
	BranchOverride   string `json:"branch_override"`
	StudioHost       string `json:"studio_host"`
	StudioPort       int    `json:"studio_port"`
	EnableStudio     bool   `json:"enable_studio"`
	DataDir          string `json:"data_dir"`
}

// DefaultConfig returns sane defaults for local development.
func DefaultConfig() *Config {
	upstreamPass := os.Getenv("BRANCHDB_PG_PASSWORD")
	if upstreamPass == "" {
		upstreamPass = os.Getenv("PGPASSWORD")
	}

	upstreamUser := os.Getenv("BRANCHDB_PG_USER")
	if upstreamUser == "" {
		upstreamUser = os.Getenv("PGUSER")
		if upstreamUser == "" {
			upstreamUser = "postgres"
		}
	}

	upstreamHost := os.Getenv("BRANCHDB_PG_HOST")
	if upstreamHost == "" {
		upstreamHost = os.Getenv("PGHOST")
		if upstreamHost == "" {
			upstreamHost = "127.0.0.1"
		}
	}

	upstreamPort := 5432
	if pStr := os.Getenv("BRANCHDB_PG_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			upstreamPort = p
		}
	} else if pStr := os.Getenv("PGPORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			upstreamPort = p
		}
	}

	proxyPort := 5433
	if pStr := os.Getenv("BRANCHDB_PROXY_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			proxyPort = p
		}
	}

	studioPort := 9550
	if pStr := os.Getenv("BRANCHDB_STUDIO_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			studioPort = p
		}
	}

	namespace := os.Getenv("BRANCHDB_NAMESPACE")
	seedFile := os.Getenv("BRANCHDB_SEED_FILE")
	if seedFile == "" {
		seedFile = ".branchdb/seed.sql"
	}

	return &Config{
		ProxyHost:        "127.0.0.1",
		ProxyPort:        proxyPort,
		UpstreamHost:     upstreamHost,
		UpstreamPort:     upstreamPort,
		UpstreamUser:     upstreamUser,
		UpstreamPassword: upstreamPass,
		UpstreamSSLMode:  "disable",
		BaseDatabase:     "branchdb_main",
		BranchPrefix:     "branchdb_",
		Namespace:        namespace,
		SeedFile:         seedFile,
		RepoPath:         ".",
		BranchOverride:   os.Getenv("BRANCHDB_BRANCH"),
		StudioHost:       "127.0.0.1",
		StudioPort:       studioPort,
		EnableStudio:     true,
		DataDir:          ".branchdb",
	}
}

// EffectivePrefix returns the active prefix considering any configured namespace.
func (c *Config) EffectivePrefix() string {
	if c.Namespace != "" {
		return c.Namespace + "_" + c.BranchPrefix
	}
	return c.BranchPrefix
}

// EffectiveBaseDatabase returns the effective base database name.
func (c *Config) EffectiveBaseDatabase() string {
	if c.Namespace != "" {
		return c.Namespace + "_" + c.BaseDatabase
	}
	return c.BaseDatabase
}

// ProxyAddr returns the proxy listen address string.
func (c *Config) ProxyAddr() string {
	return fmt.Sprintf("%s:%d", c.ProxyHost, c.ProxyPort)
}

// UpstreamAddr returns the upstream target address string.
func (c *Config) UpstreamAddr() string {
	return fmt.Sprintf("%s:%d", c.UpstreamHost, c.UpstreamPort)
}

// StudioAddr returns the studio listen address string.
func (c *Config) StudioAddr() string {
	return fmt.Sprintf("%s:%d", c.StudioHost, c.StudioPort)
}

// UpstreamDSN formats a PostgreSQL connection string for upstream admin tasks.
func (c *Config) UpstreamDSN(database string) string {
	if database == "" {
		database = "postgres"
	}
	if c.UpstreamPassword != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			c.UpstreamUser, c.UpstreamPassword, c.UpstreamHost, c.UpstreamPort, database, c.UpstreamSSLMode)
	}
	return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s",
		c.UpstreamUser, c.UpstreamHost, c.UpstreamPort, database, c.UpstreamSSLMode)
}

// ProxyDSN formats a PostgreSQL connection string pointing to the BranchDB proxy.
func (c *Config) ProxyDSN(database string) string {
	if database == "" {
		database = "postgres"
	}
	if c.UpstreamPassword != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			c.UpstreamUser, c.UpstreamPassword, c.ProxyHost, c.ProxyPort, database)
	}
	return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=disable",
		c.UpstreamUser, c.ProxyHost, c.ProxyPort, database)
}
