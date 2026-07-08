package audit

import (
	"context"
	"database/sql"
	"encoding/json"

	"orion/internal/flags"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Append(ctx context.Context, e Entry) error {
	beforeJSON, err := json.Marshal(e.Before)
	if err != nil {
		return err
	}
	afterJSON, err := json.Marshal(e.After)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_log (flag_key, action, before_state, after_state, created_at)
		VALUES ($1, $2, $3, $4, now())
	`, e.FlagKey, string(e.Action), string(beforeJSON), string(afterJSON))
	return err
}

func (s *PostgresStore) List(ctx context.Context, flagKey string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, flag_key, action, before_state, after_state, created_at
		FROM audit_log WHERE flag_key = $1 ORDER BY created_at DESC
	`, flagKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Entry
	for rows.Next() {
		var e Entry
		var actionStr string
		var beforeJSON, afterJSON []byte
		if err := rows.Scan(&e.ID, &e.FlagKey, &actionStr, &beforeJSON, &afterJSON, &e.Timestamp); err != nil {
			return nil, err
		}
		e.Action = Action(actionStr)
		if len(beforeJSON) > 0 && string(beforeJSON) != "null" {
			var f flags.Flag
			if err := json.Unmarshal(beforeJSON, &f); err != nil {
				return nil, err
			}
			e.Before = &f
		}
		if len(afterJSON) > 0 && string(afterJSON) != "null" {
			var f flags.Flag
			if err := json.Unmarshal(afterJSON, &f); err != nil {
				return nil, err
			}
			e.After = &f
		}
		result = append(result, e)
	}
	return result, rows.Err()
}