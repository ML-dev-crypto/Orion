package cluster

import (
	"encoding/json"
	"errors"
	"net/http"

	"orion/internal/flags"
	"orion/internal/store"
)

// RPCHandler serves the internal, node-to-node cluster RPC endpoints
// (/internal/cluster/...) — separate from api.Handler, which serves
// the public /flags/* API. A follower receiving a replicated write
// writes DIRECTLY to its local store.Storage, bypassing Manager's
// leader-check and FlagWriter's audit logging — audit history stays
// scoped to the leader for this stage.
type RPCHandler struct {
	Store   store.Storage
	Cluster *Cluster
}

func NewRPCHandler(s store.Storage, c *Cluster) *RPCHandler {
	return &RPCHandler{Store: s, Cluster: c}
}

func (h *RPCHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/cluster/replicate", h.replicate)
	mux.HandleFunc("DELETE /internal/cluster/replicate/{key}", h.replicateDelete)
	mux.HandleFunc("POST /internal/cluster/heartbeat", h.heartbeat)
}

func (h *RPCHandler) replicate(w http.ResponseWriter, r *http.Request) {
	var f flags.Flag
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if f.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	if err := h.Store.Put(r.Context(), f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RPCHandler) replicateDelete(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	err := h.Store.Delete(r.Context(), key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// heartbeat records that the leader is alive, as far as this node can
// tell. No leader-check here on purpose — verifying the caller's
// identity is out of scope until a later stage introduces terms.
func (h *RPCHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	h.Cluster.RecordHeartbeat()
	w.WriteHeader(http.StatusNoContent)
}
