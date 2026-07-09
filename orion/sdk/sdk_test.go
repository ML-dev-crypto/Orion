package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEvaluate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/flags/my_flag/evaluate" || r.Method != "POST" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req evalRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.UserID != "alice" {
			t.Errorf("got user_id=%q, want alice", req.UserID)
		}
		json.NewEncoder(w).Encode(evalResponse{Key: "my_flag", Enabled: true})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	enabled, err := c.Evaluate(context.Background(), "my_flag", "alice", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Error("expected enabled=true")
	}
}

func TestIsEnabled_FailsSafeOnServerDown(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", WithTimeout(500*time.Millisecond))
	got := c.IsEnabled(context.Background(), "any_flag", "alice", nil, true)
	if got != true {
		t.Errorf("IsEnabled with server down: got %v, want fallback default (true)", got)
	}
	got = c.IsEnabled(context.Background(), "any_flag", "alice", nil, false)
	if got != false {
		t.Errorf("IsEnabled with server down: got %v, want fallback default (false)", got)
	}
}

func TestIsEnabled_FailsSafeOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got := c.IsEnabled(context.Background(), "any_flag", "alice", nil, true)
	if got != true {
		t.Errorf("IsEnabled on 500: got %v, want fallback default (true)", got)
	}
}

func TestIsEnabled_FailsSafeOnFlagNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "flag not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got := c.IsEnabled(context.Background(), "does_not_exist", "alice", nil, false)
	if got != false {
		t.Errorf("IsEnabled on 404: got %v, want fallback default (false)", got)
	}
}

func TestIsEnabled_ReturnsRealValueWhenHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(evalResponse{Key: "my_flag", Enabled: true})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got := c.IsEnabled(context.Background(), "my_flag", "alice", nil, false)
	if got != true {
		t.Error("IsEnabled ignored the real server response and returned the default instead")
	}
}

func TestIsEnabled_FailsSafeOnTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		json.NewEncoder(w).Encode(evalResponse{Key: "my_flag", Enabled: true})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, WithTimeout(50*time.Millisecond))
	got := c.IsEnabled(context.Background(), "my_flag", "alice", nil, false)
	if got != false {
		t.Errorf("IsEnabled on timeout: got %v, want fallback default (false)", got)
	}
}

func TestIsEnabled_FailsSafeOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{this is not valid json`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	got := c.IsEnabled(context.Background(), "my_flag", "alice", nil, true)
	if got != true {
		t.Errorf("IsEnabled on malformed JSON: got %v, want fallback default (true)", got)
	}
}

func TestEvaluate_PropagatesErrorOnServerDown(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", WithTimeout(200*time.Millisecond))
	_, err := c.Evaluate(context.Background(), "any_flag", "alice", nil)
	if err == nil {
		t.Error("Evaluate() with server down: expected a non-nil error, got nil")
	}
}

func TestEvaluate_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(evalResponse{Key: "my_flag", Enabled: false})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	enabled, err := c.Evaluate(context.Background(), "my_flag", "alice", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Error("expected enabled=false, got true — Evaluate may be ignoring the real server response")
	}
}

func TestIsEnabled_ReturnsFalseWhenServerSaysFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(evalResponse{Key: "my_flag", Enabled: false})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	// default=true here on purpose: if IsEnabled returns false, we know
	// it's the real server value, not a coincidental match with a
	// hardcoded/buggy always-true implementation.
	got := c.IsEnabled(context.Background(), "my_flag", "alice", nil, true)
	if got != false {
		t.Error("IsEnabled returned true when server explicitly said enabled=false")
	}
}

func TestGet_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "flag not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Get(context.Background(), "missing")
	if err != ErrFlagNotFound {
		t.Errorf("got err=%v, want ErrFlagNotFound", err)
	}
}

func TestPut_SendsCorrectBodyAndReturnsSaved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/flags" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var f Flag
		json.NewDecoder(r.Body).Decode(&f)
		if f.Key != "new_flag" || f.RolloutPct != 50 {
			t.Errorf("unexpected body: %+v", f)
		}
		f.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		json.NewEncoder(w).Encode(f)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	saved, err := c.Put(context.Background(), Flag{Key: "new_flag", Enabled: true, RolloutPct: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.UpdatedAt.IsZero() {
		t.Error("expected server-assigned UpdatedAt to come back populated")
	}
}

func TestDelete_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/flags/some_flag" {
			t.Errorf("unexpected path: %s, want /flags/some_flag", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Delete(context.Background(), "some_flag"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestList_ReturnsAllFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Flag{{Key: "a"}, {Key: "b"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	flags, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(flags) != 2 {
		t.Fatalf("got %d flags, want 2", len(flags))
	}
	if flags[0].Key != "a" || flags[1].Key != "b" {
		t.Errorf("got keys [%q, %q], want [\"a\", \"b\"] — content or order wrong, not just count", flags[0].Key, flags[1].Key)
	}
}

func TestAudit_ReturnsEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]AuditEntry{
			{ID: 1, FlagKey: "my_flag", Action: AuditCreate},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	entries, err := c.Audit(context.Background(), "my_flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != AuditCreate {
		t.Errorf("unexpected entries: %+v", entries)
	}
}
