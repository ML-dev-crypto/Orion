package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	_ "github.com/lib/pq"

	"orion/internal/flags"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
    return &PostgresStore{
        db: db,
    }
}

func (s *PostgresStore) Get(ctx context.Context, key string) (flags.Flag, error) {
	var f flags.Flag
	var rulesJSON []byte
	var startAt, endAt sql.NullTime

	row := s.db.QueryRowContext(ctx,
		`SELECT key, enabled, rollout_pct, description, targeting_rules, start_at, end_at, updated_at
		 FROM flags WHERE key = $1`, key)

	err := row.Scan(&f.Key, &f.Enabled, &f.RolloutPct, &f.Description, &rulesJSON, &startAt, &endAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return flags.Flag{}, ErrNotFound
	}
	if err != nil {
		return flags.Flag{}, err
	}

	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &f.TargetingRules); err != nil {
			return flags.Flag{}, err
		}
	}
	f.Schedule = ScheduleFromNullable(startAt, endAt)
	return f, nil
}

func (s *PostgresStore) Put(ctx context.Context, f flags.Flag) error {
	rulesJSON, err := json.Marshal(f.TargetingRules)
	if err != nil {
		return err
	}

	startAt, endAt := NullableFromSchedule(f.Schedule)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO flags (key, enabled, rollout_pct, description, targeting_rules, start_at, end_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (key) DO UPDATE
		SET enabled = $2, rollout_pct = $3, description = $4, targeting_rules = $5,
		    start_at = $6, end_at = $7, updated_at = now()
	`, f.Key, f.Enabled, f.RolloutPct, f.Description, string(rulesJSON), startAt, endAt)
	return err
}

func (s *PostgresStore) Delete(ctx context.Context, key string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM flags WHERE key = $1`, key)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) List(ctx context.Context) ([]flags.Flag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, enabled, rollout_pct, description, targeting_rules, start_at, end_at, updated_at FROM flags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []flags.Flag
	for rows.Next() {
		var f flags.Flag
		var rulesJSON []byte
		var startAt, endAt sql.NullTime
		if err := rows.Scan(&f.Key, &f.Enabled, &f.RolloutPct, &f.Description, &rulesJSON, &startAt, &endAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if len(rulesJSON) > 0 {
			if err := json.Unmarshal(rulesJSON, &f.TargetingRules); err != nil {
				return nil, err
			}
		}
		f.Schedule = ScheduleFromNullable(startAt, endAt)
		result = append(result, f)
	}
	return result, rows.Err()
}

// --- helpers converting between *flags.Schedule and sql.NullTime ---

func ScheduleFromNullable(startAt, endAt sql.NullTime) *flags.Schedule {
	if !startAt.Valid && !endAt.Valid {
		return nil
	}
	sched := &flags.Schedule{}
	if startAt.Valid {
		t := startAt.Time
		sched.StartAt = &t
	}
	if endAt.Valid {
		t := endAt.Time
		sched.EndAt = &t
	}
	return sched
}

func NullableFromSchedule(s *flags.Schedule) (startAt, endAt sql.NullTime) {
	if s == nil {
		return sql.NullTime{}, sql.NullTime{}
	}
	if s.StartAt != nil {
		startAt = sql.NullTime{Time: *s.StartAt, Valid: true}
	}
	if s.EndAt != nil {
		endAt = sql.NullTime{Time: *s.EndAt, Valid: true}
	}
	return startAt, endAt
}