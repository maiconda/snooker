package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"snooker/lobby/internal/config"
	"snooker/lobby/internal/database"
	"snooker/lobby/internal/nats"
	"snooker/lobby/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("falha ao carregar config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Connect(ctx, cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("falha ao conectar banco: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(ctx, db); err != nil {
		log.Fatalf("falha ao executar migrations: %v", err)
	}

	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		log.Fatalf("falha ao conectar NATS: %v", err)
	}
	defer nc.Close()

	s, err := server.NewServer(cfg, db)
	if err != nil {
		log.Fatalf("falha ao criar servidor: %v", err)
	}

	fmt.Println("lobby service pronto")
	if err := s.Start(); err != nil {
		log.Fatalf("falha ao iniciar servidor: %v", err)
	}
}
