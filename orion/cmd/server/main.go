package main

import (
	"database/sql"
	"log"
	"net/http"

	_ "github.com/lib/pq"

	"orion/internal/api"
	"orion/internal/audit"
	"orion/internal/service"
	"orion/internal/store"
)

func main() {
	db, err := sql.Open(
		"postgres",
		"postgres://orion:orion@localhost:5432/orion?sslmode=disable",
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}
	backend := store.NewPostgresStore(db)

	auditBackend := audit.NewPostgresStore(db)

	writer := service.NewPostgresFlagWriter(db)

	h := api.NewHandler(
		backend,
		auditBackend,
		writer,
	)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", h.Routes()))
}
