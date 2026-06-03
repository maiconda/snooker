package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"snooker/internal/config"
	"snooker/internal/database"
	"snooker/internal/server"
	"snooker/internal/storage"
)

func main() {
	// 1. Carrega configuração
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Erro ao carregar configurações: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2. Conecta ao banco de dados com retry-loop resiliente
	var dbPool *pgxpool.Pool
	var connErr error
	for i := 1; i <= 3; i++ {
		fmt.Printf("Tentativa de conexão com o banco de dados (%d/3)...\n", i)
		dbPool, connErr = database.Connect(ctx, cfg.DatabaseURL())
		if connErr == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if connErr != nil {
		fmt.Printf("Erro fatal: não foi possível conectar ao banco de dados: %v\n", connErr)
		os.Exit(1)
	}
	defer dbPool.Close()

	// 3. Executa as migrations de banco automaticamente
	fmt.Println("Executando migrations do banco de dados...")
	if err := database.RunMigrations(ctx, dbPool); err != nil {
		fmt.Printf("Erro ao executar migrations: %v\n", err)
		os.Exit(1)
	}

	// 4. Inicializa o serviço de Storage (MinIO)
	fmt.Println("Conectando ao serviço de storage (MinIO)...")
	store, err := storage.NewStorageService(
		cfg.StorageEndpoint,
		cfg.StorageAccessKey,
		cfg.StorageSecretKey,
		cfg.StorageUseSSL,
		cfg.StorageBucket,
	)
	if err != nil {
		fmt.Printf("Erro ao configurar Storage: %v\n", err)
		os.Exit(1)
	}

	// Garante que o bucket do MinIO existe
	if err := store.EnsureBucket(ctx); err != nil {
		fmt.Printf("Erro ao validar bucket de avatares: %v\n", err)
	}

	// 5. Inicializa o servidor HTTP
	srv, err := server.NewServer(cfg, dbPool, store)
	if err != nil {
		fmt.Printf("Erro ao configurar servidor: %v\n", err)
		os.Exit(1)
	}

	// 6. Inicia escuta HTTP
	if err := srv.Start(); err != nil {
		fmt.Printf("Erro ao iniciar servidor HTTP: %v\n", err)
		os.Exit(1)
	}
}
