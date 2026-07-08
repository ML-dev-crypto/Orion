package main

import (
	"log"
	"net/http"

	"orion/internal/api"
	"orion/internal/store"
)

func main() {
	backend, err := store.NewPostgresStore(
		"postgres://orion:orion@localhost:5432/orion?sslmode=disable",
	)
	if err != nil {
		log.Fatal(err)
	}

	h := api.NewHandler(backend)

	log.Println("Orion listening on :8080")
	if err := http.ListenAndServe(":8080", h.Routes()); err != nil {
		log.Fatal(err)
	}
}
