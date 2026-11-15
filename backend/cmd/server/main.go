package main

import (
	"log"

	"prd-assistant/internal/api"
	"prd-assistant/internal/config"
	"prd-assistant/internal/store"
	"prd-assistant/internal/templates"
)

func main() {
	cfg := config.Load()
	tpls, err := templates.Load()
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	var st store.Store
	switch cfg.Store {
	case "memory":
		log.Println("using in-memory store (data is lost on restart)")
		st = store.NewMemory()
	default:
		m, err := store.NewMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Fatalf("store: %v", err)
		}
		st = m
	}

	r := api.New(cfg, st, tpls)
	log.Printf("PRD Assistant API on :%s  (Swagger UI: /swagger)", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
