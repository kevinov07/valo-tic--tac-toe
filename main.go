package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/jackc/pgx/v5/stdlib"

	"valo-tic-tac-toe-backend/internal/handler"
	"valo-tic-tac-toe-backend/internal/repository"
	"valo-tic-tac-toe-backend/internal/service"
)

func main() {
	addr := envOrDefault("ADDR", ":8080")
	databaseURL := envOrDefault(
		"DATABASE_URL",
		"postgres://valo:valo_dev_password@localhost:5432/valo_tic_tac_toe?sslmode=disable",
	)

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("abrir conexión a postgres: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("conectar a postgres: %v", err)
	}

	playerRepo := repository.NewPlayerRepository(db)
	engine := service.NewGameEngine(playerRepo, nil)
	gameHandler := handler.NewGameHandler(engine)
	playerHandler := handler.NewPlayerHandler(playerRepo)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api", func(r chi.Router) {
		r.Get("/players", playerHandler.Search)
		r.Post("/games", gameHandler.CreateGame)
		r.Route("/games/{gameID}", func(r chi.Router) {
			r.Get("/", gameHandler.GetGame)
			r.Post("/guess", gameHandler.Guess)
		})
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("servidor escuchando en %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor HTTP: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	log.Println("apagando servidor...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown del servidor: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
