package config

import "os"

type Config struct {
	Port      string
	Store     string // mysql | memory
	MySQLDSN  string
	OllamaURL string
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	return Config{
		Port:      get("PORT", "8080"),
		Store:     get("STORE", "mysql"),
		MySQLDSN:  get("MYSQL_DSN", "prd:prdpass@tcp(localhost:3306)/prd_assistant?parseTime=true&charset=utf8mb4"),
		OllamaURL: get("OLLAMA_URL", "http://localhost:11434"),
	}
}
