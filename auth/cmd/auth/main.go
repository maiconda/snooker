package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"snooker/auth/internal/config"
	"snooker/auth/internal/database"
	"snooker/auth/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("erro ao carregar configuracao: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var dbPool *pgxpool.Pool
	var connErr error
	for i := 1; i <= 3; i++ {
		fmt.Printf("tentativa de conexao com o banco de auth (%d/3)...\n", i)
		dbPool, connErr = database.Connect(ctx, cfg.DatabaseURL())
		if connErr == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if connErr != nil {
		fmt.Printf("erro fatal: nao foi possivel conectar ao banco de auth: %v\n", connErr)
		os.Exit(1)
	}
	defer dbPool.Close()

	fmt.Println("executando migrations do auth service...")
	if err := database.RunMigrations(ctx, dbPool); err != nil {
		fmt.Printf("erro ao executar migrations de auth: %v\n", err)
		os.Exit(1)
	}

	srv, err := server.NewServer(cfg, dbPool)
	if err != nil {
		fmt.Printf("erro ao configurar auth service: %v\n", err)
		os.Exit(1)
	}

	if err := srv.Start(); err != nil {
		fmt.Printf("erro ao iniciar auth service: %v\n", err)
		os.Exit(1)
	}
}
