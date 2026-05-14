package main

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	Addr              string
	SQLitePath        string
	UploadDir         string
	ActualAPIURL      string
	ActualAPIKey      string
	ActualBudgetSyncID string
}

func loadConfig() Config {
	loadDotEnv(".env")

	cfg := Config{
		Addr:               getenv("ADDR", "127.0.0.1:8080"),
		SQLitePath:         getenv("SQLITE_PATH", "./banktoactual.db"),
		UploadDir:          getenv("UPLOAD_DIR", "./uploads"),
		ActualAPIURL:       getenv("ACTUAL_API_URL", "http://localhost:5007"),
		ActualAPIKey:       getenv("ACTUAL_API_KEY", ""),
		ActualBudgetSyncID: getenv("ACTUAL_BUDGET_SYNC_ID", ""),
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv reads key=value pairs from path into the process environment.
// Existing env vars take precedence (not overwritten).
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
