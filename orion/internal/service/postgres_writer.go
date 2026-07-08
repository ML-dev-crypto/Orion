package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"orion/internal/audit"
	"orion/internal/flags"
	"orion/internal/store"
)

// PostgresFlagWriter is the ONLY place in the codebase that knows
// about pg_advisory_xact_lock or ON CONFLICT — contained here rather
// than leaking into store.PostgresStore or the HTTP handler.
type PostgresFlagWriter struct {
	db *sql.DB
}

func NewPostgresFlagWriter(db *sql.DB) *PostgresFlagWriter {
	return &PostgresFlagWriter{db: db}
}

func (w *PostgresFlagWriter) Upsert(ctx context.Context, f flags.Flag) (WriteResult, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return WriteResult{}, err
	}
	defer tx.Rollback()

	// Serializes every writer touching this key, whether or not a row
	// exists yet — closes both the update race AND the create race
	// (which "SELECT ... FOR UPDATE" alone can't catch, since there's
	// no row to lock before the first insert).
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, f.Key); err != nil {
		return WriteResult{}, err
	}

	existing, existed, err := getFlagTx(ctx, tx, f.Key)
	if err != nil {
		return WriteResult{}, err
	}

	if err := upsertFlagTx(ctx, tx, f); err != nil {
		return WriteResult{}, err
	}

	saved, _, err := getFlagTx(ctx, tx, f.Key)
	if err != nil {
		return WriteResult{}, err
	}

	result := WriteResult{Flag: saved}
	if existed {
		e := existing
		result.Existing = &e
		result.Changed = !flags.Equal(existing, saved)
	} else {
		result.Changed = true
	}

	if result.Changed {
		action := audit.ActionUpdate
		if !existed {
			action = audit.ActionCreate
		}
		if err := insertAuditTx(ctx, tx, audit.Entry{
			FlagKey:   f.Key,
			Action:    action,
			Before:    result.Existing,
			After:     &saved,
			Timestamp: time.Now().UTC(),
		}); err != nil {
			return WriteResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return WriteResult{}, err
	}
	return result, nil
}

func (w *PostgresFlagWriter) Delete(ctx context.Context, key string) (flags.Flag, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return flags.Flag{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key); err != nil {
		return flags.Flag{}, err
	}

	existing, existed, err := getFlagTx(ctx, tx, key)
	if err != nil {
		return flags.Flag{}, err
	}
	if !existed {
		return flags.Flag{}, store.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM flags WHERE key = $1`, key); err != nil {
		return flags.Flag{}, err
	}

	if err := insertAuditTx(ctx, tx, audit.Entry{
		FlagKey:   key,
		Action:    audit.ActionDelete,
		Before:    &existing,
		Timestamp: time.Now().UTC(),
	}); err != nil {
		return flags.Flag{}, err
	}

	if err := tx.Commit(); err != nil {
		return flags.Flag{}, err
	}
	return existing, nil
}

func getFlagTx(ctx context.Context, tx *sql.Tx, key string) (flags.Flag, bool, error) {
	var f flags.Flag
	var rulesJSON []byte
	var startAt, endAt sql.NullTime

	row := tx.QueryRowContext(ctx,
		`SELECT key, enabled, rollout_pct, description, targeting_rules, start_at, end_at, updated_at
		 FROM flags WHERE key = $1 FOR UPDATE`, key)

	err := row.Scan(&f.Key, &f.Enabled, &f.RolloutPct, &f.Description, &rulesJSON, &startAt, &endAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return flags.Flag{}, false, nil
	}
	if err != nil {
		return flags.Flag{}, false, err
	}
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &f.TargetingRules); err != nil {
			return flags.Flag{}, false, err
		}
	}
	f.Schedule = store.ScheduleFromNullable(startAt, endAt)
	return f, true, nil
}

func upsertFlagTx(ctx context.Context, tx *sql.Tx, f flags.Flag) error {
	rulesJSON, err := json.Marshal(f.TargetingRules)
	if err != nil {
		return err
	}
	startAt, endAt := store.NullableFromSchedule(f.Schedule)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO flags (key, enabled, rollout_pct, description, targeting_rules, start_at, end_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (key) DO UPDATE
		SET enabled = $2, rollout_pct = $3, description = $4, targeting_rules = $5,
		    start_at = $6, end_at = $7, updated_at = now()
	`, f.Key, f.Enabled, f.RolloutPct, f.Description, string(rulesJSON), startAt, endAt)
	return err
}

func insertAuditTx(ctx context.Context, tx *sql.Tx, e audit.Entry) error {
	beforeJSON, err := json.Marshal(e.Before)
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(e.After)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_log (flag_key, action, before_state, after_state, created_at)
		VALUES ($1, $2, $3, $4, now())
	`, e.FlagKey, string(e.Action), string(beforeJSON), string(afterJSON))
	return err
}