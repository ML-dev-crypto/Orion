package main

import (
	"database/sql"
	"log"
	"net/http"

	_ "github.com/lib/pq"

	"orion/internal/api"
	"orion/internal/audit"
	"orion/internal/cluster"
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

	// Single-node cluster for now: this node is the only member and
	// is always the leader. Stage 2+ introduces real peers,
	// replication, and eventually real election — none of that
	// changes anything above this line.
	self := cluster.Node{ID: "node-1", Address: "localhost:8080"}
	c, err := cluster.NewCluster(self, []cluster.Node{self}, self.ID)
	if err != nil {
		log.Fatal(err)
	}
	rpcClient := cluster.NewClient()
	manager := cluster.NewManager(c, writer, rpcClient)
	rpcHandler := cluster.NewRPCHandler(backend, c)

	h := api.NewHandler(backend, auditBackend, manager)
	mux := h.Routes()
	rpcHandler.RegisterRoutes(mux)

	log.Println("Orion listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
