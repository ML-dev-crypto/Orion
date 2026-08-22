package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"orion/internal/audit"
	"orion/internal/cluster"
	"orion/internal/flags"
	"orion/internal/store"
)

type Handler struct {
	Store   store.Storage
	Audit   audit.Store
	Cluster *cluster.Manager
}

func NewHandler(s store.Storage, a audit.Store, c *cluster.Manager) *Handler {
	return &Handler{Store: s, Audit: a, Cluster: c}
}

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /flags", h.createOrUpdate)
	mux.HandleFunc("GET /flags", h.list)
	mux.HandleFunc("GET /flags/{key}", h.get)
	mux.HandleFunc("DELETE /flags/{key}", h.delete)
	mux.HandleFunc("POST /flags/{key}/evaluate", h.evaluate)
	mux.HandleFunc("GET /flags/{key}/audit", h.listAudit)
	mux.HandleFunc("GET /cluster/status", h.clusterStatus)
	return mux
}

func (h *Handler) createOrUpdate(w http.ResponseWriter, r *http.Request) {
	var f flags.Flag
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if f.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	result, err := h.Cluster.Upsert(r.Context(), f)
	if err != nil {
		respondWriteErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result.Flag)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	f, err := h.Store.Get(r.Context(), key)
	if err != nil {
		respondStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	all, err := h.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, all)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, err := h.Cluster.Delete(r.Context(), key); err != nil {
		respondWriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) evaluate(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	var req flags.EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "user_id is required in the request body", http.StatusBadRequest)
		return
	}

	f, err := h.Store.Get(r.Context(), key)
	if err != nil {
		respondStoreErr(w, err)
		return
	}

	enabled := flags.Evaluate(f, req)
	writeJSON(w, http.StatusOK, flags.EvalResponse{Key: key, Enabled: enabled})
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	entries, err := h.Audit.List(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) clusterStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Cluster.Status())
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func respondStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "flag not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// respondWriteErr additionally handles cluster.ErrQuorumNotReached,
// mapped to 503: the request was valid and WAS applied to the
// leader's local store, but the cluster couldn't confirm durability
// across a majority right now — that's "the service can't currently
// guarantee what you asked for," not a client mistake (400) or an
// unexpected crash (500).
func respondWriteErr(w http.ResponseWriter, err error) {
	if errors.Is(err, cluster.ErrNotLeader) {
		http.Error(w, "this node is not the leader", http.StatusBadRequest)
		return
	}
	if errors.Is(err, cluster.ErrQuorumNotReached) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	respondStoreErr(w, err)
}
