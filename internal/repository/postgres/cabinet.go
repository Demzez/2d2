package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"telecom/internal/object"
)

func (r *Repository) ReadMe(ctx context.Context, subscriberID int64) (json.RawMessage, error) {
	const query = `SELECT json_build_object(
		'user', json_build_object('id', u.id, 'login', u.login, 'full_name', u.full_name, 'email', u.email, 'role', u.role),
		'number', json_build_object('id', s.id, 'msisdn', s.msisdn, 'account_number', s.account_number,
		'nickname', s.nickname, 'is_blocked', s.is_blocked, 'admin_blocked', s.admin_blocked, 'tariff_id', s.tariff_id))
		FROM telecom.subscribers s JOIN telecom.users u ON u.id = s.user_id WHERE s.id = $1`
	return r.ReadJSON(ctx, query, subscriberID)
}

func (r *Repository) UpdateMe(ctx context.Context, subscriberID int64, name, email *string) (json.RawMessage, error) {
	const op = op + ".UpdateMe()"
	const query = `WITH changed AS (
		UPDATE telecom.users SET full_name = COALESCE($2::text, full_name),
		email = CASE WHEN $3::text IS NULL THEN email ELSE NULLIF($3::text, '') END, updated_at = now()
		WHERE id = (SELECT user_id FROM telecom.subscribers WHERE id = $1)
		RETURNING id, login, full_name, email, role)
		SELECT json_build_object('user', row_to_json(changed)) FROM changed`
	value, err := r.UpdateJSON(ctx, query, subscriberID, name, email)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == "23505" {
			return nil, object.NewError(409, "CONFLICT", "Электронная почта уже используется")
		}
		return nil, fmt.Errorf("%s | updating user failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) UpdateNumber(ctx context.Context, subscriberID int64, nickname *string, blocked *bool) (json.RawMessage, error) {
	const op = op + ".UpdateNumber()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	var adminBlocked bool
	if err := tx.QueryRowContext(ctx, `SELECT admin_blocked FROM telecom.subscribers WHERE id = $1 FOR UPDATE`, subscriberID).Scan(&adminBlocked); err != nil {
		return nil, fmt.Errorf("%s | locking subscriber failed: %w", op, err)
	}
	if blocked != nil && !*blocked && adminBlocked {
		return nil, object.NewError(403, "ADMIN_BLOCKED", "Номер заблокирован администратором")
	}

	var data []byte
	err = tx.QueryRowContext(ctx, `WITH changed AS (UPDATE telecom.subscribers
		SET nickname = COALESCE($2::text, nickname), is_blocked = COALESCE($3::boolean, is_blocked), updated_at = now()
		WHERE id = $1 RETURNING msisdn, nickname, is_blocked, admin_blocked)
		SELECT json_build_object('number', row_to_json(changed)) FROM changed`, subscriberID, nickname, blocked).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("%s | updating number failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return json.RawMessage(data), nil
}

func (r *Repository) ReadTariffs(ctx context.Context, subscriberID int64) (json.RawMessage, error) {
	const query = `SELECT json_build_object('items', COALESCE(json_agg(json_build_object(
		'id', t.id, 'code', t.code, 'name', t.name, 'description', t.description,
		'monthly_fee_byn', t.monthly_fee_byn::text, 'included_internet_mb', t.included_internet_mb,
		'is_current', t.id = s.tariff_id) ORDER BY t.monthly_fee_byn, t.id), '[]'::json))
		FROM telecom.tariffs t CROSS JOIN telecom.subscribers s
		WHERE s.id = $1 AND (t.is_active OR t.id = s.tariff_id)`
	return r.ReadJSON(ctx, query, subscriberID)
}

func (r *Repository) UpdateTariff(ctx context.Context, subscriberID, tariffID int64) (json.RawMessage, error) {
	const op = op + ".UpdateTariff()"
	query := `WITH changed AS (UPDATE telecom.subscribers s SET tariff_id = t.id, updated_at = now()
		FROM telecom.tariffs t WHERE s.id = $1 AND t.id = $2 AND t.is_active
		RETURNING t.id, t.name, t.monthly_fee_byn)
		SELECT json_build_object('tariff', json_build_object('id', id, 'name', name,
		'monthly_fee_byn', monthly_fee_byn::text), 'effective', 'immediately') FROM changed`
	value, err := r.UpdateJSON(ctx, query, subscriberID, tariffID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Тариф не найден")
	}
	if err != nil {
		return nil, fmt.Errorf("%s | updating tariff failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) ReadServices(ctx context.Context, subscriberID int64, status, search, category string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT v.id, v.name, v.description, v.category, v.monthly_fee_byn::text AS monthly_fee_byn,
		v.quota_metric, v.quota_amount, v.is_unlimited, (ss.id IS NOT NULL) AS is_connected, ss.connected_at
		FROM telecom.services v
		LEFT JOIN telecom.subscriber_services ss ON ss.service_id = v.id AND ss.subscriber_id = $1 AND ss.disconnected_at IS NULL
		WHERE (v.is_active OR ss.id IS NOT NULL) AND
		($2 = 'all' OR ($2 = 'connected' AND ss.id IS NOT NULL) OR ($2 = 'available' AND ss.id IS NULL))
		AND ($3 = '' OR v.name ILIKE '%' || $3 || '%' OR v.description ILIKE '%' || $3 || '%')
		AND ($4 = '' OR v.category = $4)
	)
	SELECT json_build_object('items', COALESCE(json_agg(row_to_json(items) ORDER BY id), '[]'::json), 'count', count(*)) FROM items`
	return r.ReadJSON(ctx, query, subscriberID, status, search, category)
}

func (r *Repository) UpdateService(ctx context.Context, subscriberID, serviceID int64, enabled bool) (json.RawMessage, error) {
	const op = op + ".UpdateService()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	var blocked bool
	if err := tx.QueryRowContext(ctx,
		`SELECT (is_blocked OR admin_blocked) FROM telecom.subscribers WHERE id = $1 FOR UPDATE`,
		subscriberID,
	).Scan(&blocked); err != nil {
		return nil, fmt.Errorf("%s | subscriber lock failed: %w", op, err)
	}

	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT is_active FROM telecom.services WHERE id = $1`, serviceID).Scan(&active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, object.NewError(404, "NOT_FOUND", "Услуга не найдена")
		}
		return nil, fmt.Errorf("%s | service lookup failed: %w", op, err)
	}

	var existingConnectedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT connected_at FROM telecom.subscriber_services
		WHERE subscriber_id = $1 AND service_id = $2 AND disconnected_at IS NULL ORDER BY id DESC LIMIT 1`, subscriberID, serviceID).Scan(&existingConnectedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s | existing connection lookup failed: %w", op, err)
	}

	if enabled {
		if !existingConnectedAt.Valid {
			if !active {
				return nil, object.NewError(409, "CONFLICT", "Услуга недоступна")
			}
			if blocked {
				return nil, object.NewError(409, "NUMBER_BLOCKED", "Номер заблокирован")
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO telecom.subscriber_services(subscriber_id, service_id) VALUES ($1, $2)`, subscriberID, serviceID); err != nil {
				return nil, fmt.Errorf("%s | service connection failed: %w", op, err)
			}
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE telecom.subscriber_services SET disconnected_at = now()
			WHERE subscriber_id = $1 AND service_id = $2 AND disconnected_at IS NULL`, subscriberID, serviceID); err != nil {
			return nil, fmt.Errorf("%s | service disconnection failed: %w", op, err)
		}
	}

	var connectedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT connected_at FROM telecom.subscriber_services
		WHERE subscriber_id = $1 AND service_id = $2 AND disconnected_at IS NULL ORDER BY id DESC LIMIT 1`, subscriberID, serviceID).Scan(&connectedAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s | connection lookup failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	var at any
	if connectedAt.Valid {
		at = connectedAt.Time.Format(time.RFC3339)
	}
	data, _ := json.Marshal(map[string]any{"service_id": serviceID, "is_connected": connectedAt.Valid, "connected_at": at})
	return data, nil
}

func (r *Repository) ReadDashboard(ctx context.Context, subscriberID int64, month string) (json.RawMessage, error) {
	const query = `WITH sub AS (
		SELECT s.*, t.name AS tariff_name, t.monthly_fee_byn, t.included_internet_mb
		FROM telecom.subscribers s JOIN telecom.tariffs t ON t.id = s.tariff_id WHERE s.id = $1
	), used AS (
		SELECT coalesce(sum(quantity),0)::float8 AS total FROM telecom.usage_events
		WHERE subscriber_id = $1 AND metric = 'internet_mb' AND service_id IS NULL
		AND occurred_at >= (($2 || '-01')::date AT TIME ZONE 'Europe/Minsk')
		AND occurred_at < ((($2 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
	), packages AS (
		SELECT coalesce(json_agg(json_build_object(
		'service_id', v.id, 'name', v.name, 'metric', v.quota_metric,
		'is_unlimited', v.is_unlimited, 'quota_amount', v.quota_amount,
		'used_amount', coalesce(u.used, 0), 'remaining_amount',
		CASE WHEN v.is_unlimited OR v.quota_amount IS NULL THEN NULL
		ELSE greatest(v.quota_amount - coalesce(u.used,0), 0) END) ORDER BY v.id), '[]'::json) AS items
		FROM telecom.subscriber_services ss JOIN telecom.services v ON v.id = ss.service_id
		LEFT JOIN LATERAL (
		SELECT sum(e.quantity)::float8 AS used FROM telecom.usage_events e
		WHERE e.subscriber_id = $1 AND e.service_id = v.id AND e.metric = v.quota_metric
		AND e.occurred_at >= (($2 || '-01')::date AT TIME ZONE 'Europe/Minsk')
		AND e.occurred_at < ((($2 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		) u ON TRUE WHERE ss.subscriber_id = $1 AND ss.disconnected_at IS NULL AND v.quota_metric IS NOT NULL
	), unread AS (
		SELECT count(*) FILTER (WHERE kind = 'alert') AS alerts, count(*) FILTER (WHERE kind = 'message') AS messages
		FROM telecom.notifications WHERE subscriber_id = $1 AND read_at IS NULL
	), recommendations AS (
		SELECT coalesce(json_agg(json_build_object('type', 'service', 'id', id, 'title', name, 'tag', 'popular') ORDER BY id), '[]'::json) AS items
		FROM (SELECT id, name FROM telecom.services WHERE is_active AND is_featured AND id NOT IN
		(SELECT service_id FROM telecom.subscriber_services WHERE subscriber_id = $1 AND disconnected_at IS NULL)
		ORDER BY id LIMIT 3) recommended
	)
	SELECT json_build_object(
		'balance_byn', sub.balance_byn::text, 'balance_updated_at', sub.updated_at,
		'tariff', json_build_object('id', sub.tariff_id, 'name', sub.tariff_name,
		'monthly_fee_byn', sub.monthly_fee_byn::text),
		'connected_services_count', (SELECT count(*) FROM telecom.subscriber_services WHERE subscriber_id = $1 AND disconnected_at IS NULL),
		'traffic', json_build_object('internet', json_build_object('included_mb', sub.included_internet_mb,
		'used_mb', used.total, 'remaining_mb', greatest(sub.included_internet_mb - used.total,0)), 'services', packages.items),
		'recommendations', recommendations.items,
		'unread', json_build_object('alerts', unread.alerts, 'messages', unread.messages))
	FROM sub CROSS JOIN used CROSS JOIN packages CROSS JOIN unread CROSS JOIN recommendations`
	return r.ReadJSON(ctx, query, subscriberID, month)
}

func (r *Repository) ReadFavorites(ctx context.Context, subscriberID int64) (json.RawMessage, error) {
	return r.ReadJSON(ctx, `SELECT json_build_object('items', coalesce(json_agg(json_build_object('id', id, 'name', name, 'phone', phone) ORDER BY id), '[]'::json)) FROM telecom.favorite_contacts WHERE subscriber_id = $1`, subscriberID)
}

func (r *Repository) CreateFavorite(ctx context.Context, subscriberID int64, name, phone string) (json.RawMessage, error) {
	const op = op + ".CreateFavorite()"
	value, err := r.UpdateJSON(ctx, `WITH added AS (INSERT INTO telecom.favorite_contacts(subscriber_id, name, phone)
		VALUES ($1,$2,$3) RETURNING id, name, phone) SELECT row_to_json(added) FROM added`, subscriberID, name, phone)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == "23505" {
			return nil, object.NewError(409, "CONFLICT", "Номер уже добавлен")
		}
		return nil, fmt.Errorf("%s | creating favorite failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) UpdateFavorite(ctx context.Context, subscriberID, id int64, name, phone *string) (json.RawMessage, error) {
	const op = op + ".UpdateFavorite()"
	value, err := r.UpdateJSON(ctx, `WITH changed AS (UPDATE telecom.favorite_contacts
		SET name = COALESCE($3::text, name), phone = COALESCE($4::text, phone)
		WHERE subscriber_id = $1 AND id = $2 RETURNING id, name, phone)
		SELECT row_to_json(changed) FROM changed`, subscriberID, id, name, phone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Контакт не найден")
	}
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && string(pgErr.Code) == "23505" {
			return nil, object.NewError(409, "CONFLICT", "Номер уже добавлен")
		}
		return nil, fmt.Errorf("%s | updating favorite failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) DeleteFavorite(ctx context.Context, subscriberID, id int64) error {
	const op = op + ".DeleteFavorite()"
	result, err := r.db.ExecContext(ctx, `DELETE FROM telecom.favorite_contacts WHERE subscriber_id = $1 AND id = $2`, subscriberID, id)
	if err != nil {
		return fmt.Errorf("%s | deleting favorite failed: %w", op, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s | checking deleted favorite failed: %w", op, err)
	}
	if affected == 0 {
		return object.NewError(404, "NOT_FOUND", "Контакт не найден")
	}
	return nil
}

func (r *Repository) ReadNotifications(ctx context.Context, subscriberID int64, kind string, unreadOnly bool) (json.RawMessage, error) {
	const query = `WITH unread AS (SELECT
		count(*) FILTER (WHERE kind = 'alert') AS alerts,
		count(*) FILTER (WHERE kind = 'message') AS messages
		FROM telecom.notifications WHERE subscriber_id = $1 AND read_at IS NULL),
		items AS (SELECT id, kind, title, body, target_path, created_at, (read_at IS NOT NULL) AS is_read
		FROM telecom.notifications WHERE subscriber_id = $1 AND ($2 = 'all' OR kind = $2)
		AND (NOT $3 OR read_at IS NULL) ORDER BY created_at DESC, id DESC LIMIT 50)
		SELECT json_build_object('unread', json_build_object('alerts', unread.alerts, 'messages', unread.messages),
		'items', (SELECT coalesce(json_agg(row_to_json(items)), '[]'::json) FROM items)) FROM unread`
	return r.ReadJSON(ctx, query, subscriberID, kind, unreadOnly)
}

func (r *Repository) UpdateNotification(ctx context.Context, subscriberID, id int64) (json.RawMessage, error) {
	const query = `WITH changed AS (UPDATE telecom.notifications SET read_at = COALESCE(read_at, now())
		WHERE subscriber_id = $1 AND id = $2 RETURNING id) SELECT json_build_object('id', id, 'is_read', true) FROM changed`
	value, err := r.UpdateJSON(ctx, query, subscriberID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Уведомление не найдено")
	}
	return value, err
}
