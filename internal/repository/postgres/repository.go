package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"telecom/internal/config"

	_ "github.com/lib/pq"
)

const op = "2d2.internal.repository.postgres"

type Repository struct {
	db *sql.DB
}

func NewRepository(cfg *config.Config) (*Repository, error) {
	const op = op + ".NewRepository()"

	db, err := sql.Open("postgres", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("%s | database open failed: %w", op, err)
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s | database connect failed: %w", op, err)
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) ReadJSON(ctx context.Context, query string, args ...any) (json.RawMessage, error) {
	var data []byte
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&data); err != nil {
		return nil, fmt.Errorf("%s.ReadJSON() | reading JSON failed: %w", op, err)
	}
	return json.RawMessage(data), nil
}

func (r *Repository) UpdateJSON(ctx context.Context, query string, args ...any) (json.RawMessage, error) {
	return r.ReadJSON(ctx, query, args...)
}

func (r *Repository) ReadSubscriberID(ctx context.Context, userID int64) (int64, error) {
	const op = op + ".ReadSubscriberID()"
	var id int64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM telecom.subscribers WHERE user_id = $1`, userID).Scan(&id); err != nil {
		return 0, fmt.Errorf("%s | subscriber lookup failed: %w", op, err)
	}
	return id, nil
}
