package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"telecom/internal/object"

	"github.com/lib/pq"
)

type LoginRow struct {
	UserID       int64
	PasswordHash string
}

func (r *Repository) ReadLogin(ctx context.Context, login string) (LoginRow, error) {
	const op = op + ".ReadLogin()"
	var row LoginRow
	err := r.db.QueryRowContext(ctx, `SELECT id, password_hash FROM telecom.users WHERE lower(login) = lower($1)`, login).Scan(&row.UserID, &row.PasswordHash)
	if err != nil {
		return row, fmt.Errorf("%s | reading login failed: %w", op, err)
	}
	return row, nil
}

func (r *Repository) ReadPhoneUser(ctx context.Context, phone string) (int64, error) {
	const op = op + ".ReadPhoneUser()"
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM telecom.subscribers WHERE msisdn = $1`, phone).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%s | reading phone user failed: %w", op, err)
	}
	return id, nil
}

func (r *Repository) CreateOTP(ctx context.Context, phone, purpose, hash string) error {
	const op = op + ".CreateOTP()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	// Lock on the phone+purpose, including concurrent first requests.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, phone+":"+purpose); err != nil {
		return fmt.Errorf("%s | phone lock failed: %w", op, err)
	}
	var recent bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM telecom.auth_otps WHERE phone = $1 AND purpose = $2 AND created_at > now() - interval '60 seconds')`, phone, purpose).Scan(&recent); err != nil {
		return fmt.Errorf("%s | reading OTP window failed: %w", op, err)
	}
	if recent {
		return object.NewError(429, "RATE_LIMITED", "Повторный запрос кода возможен через 60 секунд")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE telecom.auth_otps SET consumed_at = now() WHERE phone = $1 AND purpose = $2 AND consumed_at IS NULL`, phone, purpose); err != nil {
		return fmt.Errorf("%s | invalidating old OTP failed: %w", op, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO telecom.auth_otps (phone, purpose, code_hash, expires_at) VALUES ($1, $2, $3, now() + interval '5 minutes')`, phone, purpose, hash); err != nil {
		return fmt.Errorf("%s | creating OTP failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return nil
}

// consumeOTP commits an invalid attempt counter; do not return an error before Commit.
func consumeOTP(ctx context.Context, tx *sql.Tx, phone, purpose, hash string) (string, error) {
	var id int64
	var stored string
	var attempts int
	var expires time.Time
	err := tx.QueryRowContext(ctx, `SELECT id, code_hash, attempts, expires_at FROM telecom.auth_otps
		WHERE phone = $1 AND purpose = $2 AND consumed_at IS NULL ORDER BY id DESC LIMIT 1 FOR UPDATE`, phone, purpose).
		Scan(&id, &stored, &attempts, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "INVALID_OTP", nil
	}
	if err != nil {
		return "", err
	}
	if time.Now().After(expires) {
		return "OTP_EXPIRED", nil
	}
	if attempts >= 5 {
		return "INVALID_OTP", nil
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hash)) != 1 {
		_, err = tx.ExecContext(ctx, `UPDATE telecom.auth_otps SET attempts = attempts + 1 WHERE id = $1`, id)
		return "INVALID_OTP", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE telecom.auth_otps SET consumed_at = now() WHERE id = $1`, id)
	return "", err
}

func (r *Repository) VerifyOTP(ctx context.Context, phone, purpose, hash string) (int64, error) {
	const op = op + ".VerifyOTP()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	code, err := consumeOTP(ctx, tx, phone, purpose, hash)
	if err != nil {
		return 0, fmt.Errorf("%s | OTP verification failed: %w", op, err)
	}
	var userID int64
	if code == "" {
		if err = tx.QueryRowContext(ctx, `SELECT user_id FROM telecom.subscribers WHERE msisdn = $1`, phone).Scan(&userID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				code = "INVALID_OTP"
			} else {
				return 0, fmt.Errorf("%s | account lookup failed: %w", op, err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	if code != "" {
		return 0, object.NewError(401, code, "Неверный или истёкший код")
	}
	return userID, nil
}

func (r *Repository) CreateAccount(ctx context.Context, login, fullName, email, phone, passwordHash, otpHash string) (json.RawMessage, int64, error) {
	const op = op + ".CreateAccount()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	code, err := consumeOTP(ctx, tx, phone, "register", otpHash)
	if err != nil {
		return nil, 0, fmt.Errorf("%s | OTP verification failed: %w", op, err)
	}
	if code != "" {
		if err := tx.Commit(); err != nil {
			return nil, 0, fmt.Errorf("%s | OTP commit failed: %w", op, err)
		}
		return nil, 0, object.NewError(401, code, "Неверный или истёкший код")
	}

	var userID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO telecom.users (login, full_name, email, password_hash, privacy_accepted_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, now()) RETURNING id`, login, fullName, email, passwordHash).Scan(&userID)
	if err != nil {
		return nil, 0, uniqueOrError(op, err)
	}
	var tariffID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM telecom.tariffs WHERE code = 'comfort-s' AND is_active`).Scan(&tariffID); err != nil {
		return nil, 0, fmt.Errorf("%s | starter tariff lookup failed: %w", op, err)
	}
	var subID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO telecom.subscribers(user_id, msisdn, account_number, tariff_id, nickname)
		VALUES ($1, $2, '2D2-' || lpad(nextval('telecom.account_numbers_seq')::text, 6, '0'), $3, 'Мой номер') RETURNING id`, userID, phone, tariffID).Scan(&subID)
	if err != nil {
		return nil, 0, uniqueOrError(op, err)
	}

	var data []byte
	err = tx.QueryRowContext(ctx, `SELECT json_build_object(
		'user', json_build_object('id', u.id, 'login', u.login, 'full_name', u.full_name),
		'account', json_build_object('number', s.msisdn, 'account_number', s.account_number))
		FROM telecom.users u JOIN telecom.subscribers s ON s.user_id = u.id WHERE u.id = $1`, userID).Scan(&data)
	if err != nil {
		return nil, 0, fmt.Errorf("%s | reading created account failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return data, userID, nil
}

func uniqueOrError(op string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "23505" {
		return object.NewError(409, "CONFLICT", "Логин, электронная почта или телефон уже зарегистрированы")
	}
	return fmt.Errorf("%s | database write failed: %w", op, err)
}

func (r *Repository) CreateSession(ctx context.Context, userID int64, hash string) error {
	const op = op + ".CreateSession()"
	_, err := r.db.ExecContext(ctx, `INSERT INTO telecom.auth_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + interval '30 days')`, userID, hash)
	if err != nil {
		return fmt.Errorf("%s | creating session failed: %w", op, err)
	}
	return nil
}

func (r *Repository) UpdateSession(ctx context.Context, oldHash, newHash string) (int64, error) {
	const op = op + ".UpdateSession()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()
	var userID int64
	err = tx.QueryRowContext(ctx, `UPDATE telecom.auth_sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now() RETURNING user_id`, oldHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, object.NewError(401, "UNAUTHORIZED", "Сессия недействительна")
	}
	if err != nil {
		return 0, fmt.Errorf("%s | session rotation failed: %w", op, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO telecom.auth_sessions(user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + interval '30 days')`, userID, newHash); err != nil {
		return 0, fmt.Errorf("%s | creating rotated session failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return userID, nil
}

func (r *Repository) DeleteSession(ctx context.Context, hash string) error {
	const op = op + ".DeleteSession()"
	_, err := r.db.ExecContext(ctx, `UPDATE telecom.auth_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	if err != nil {
		return fmt.Errorf("%s | revoking session failed: %w", op, err)
	}
	return nil
}
