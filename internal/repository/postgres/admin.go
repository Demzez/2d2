package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"telecom/internal/object"

	"github.com/lib/pq"
)

type AdminTariffInput struct {
	Code               string
	Name               string
	Description        string
	MonthlyFeeBYN      string
	IncludedInternetMB int
	IsFeatured         bool
	IsActive           bool
}

type AdminTariffPatch struct {
	Code               *string
	Name               *string
	Description        *string
	MonthlyFeeBYN      *string
	IncludedInternetMB *int
	IsFeatured         *bool
	IsActive           *bool
}

type AdminServiceRow struct {
	ID            int64   `json:"id"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	MonthlyFeeBYN string  `json:"monthly_fee_byn"`
	QuotaMetric   *string `json:"quota_metric"`
	QuotaAmount   *int    `json:"quota_amount"`
	IsUnlimited   bool    `json:"is_unlimited"`
	IsFeatured    bool    `json:"is_featured"`
	IsActive      bool    `json:"is_active"`
}

func (r *Repository) ReadAdminOverview(ctx context.Context) (json.RawMessage, error) {
	const query = `SELECT json_build_object(
		'total_subscribers', (SELECT count(*) FROM telecom.subscribers),
		'blocked_numbers', (SELECT count(*) FROM telecom.subscribers WHERE is_blocked OR admin_blocked),
		'negative_balance_numbers', (SELECT count(*) FROM telecom.subscribers WHERE balance_byn < 0),
		'active_service_connections', (SELECT count(*) FROM telecom.subscriber_services WHERE disconnected_at IS NULL),
		'active_catalog_services', (SELECT count(*) FROM telecom.services WHERE is_active)
	)`
	return r.ReadJSON(ctx, query)
}

func (r *Repository) ReadAdminSubscribers(ctx context.Context, search, status string, page, limit int) (json.RawMessage, error) {
	const query = `WITH filtered AS (
		SELECT s.id, s.balance_byn, s.is_blocked, s.admin_blocked, s.msisdn, s.account_number,
		       u.id AS user_id, u.login, u.full_name,
		       t.id AS tariff_id, t.name AS tariff_name
		FROM telecom.subscribers s
		JOIN telecom.users u ON u.id = s.user_id
		JOIN telecom.tariffs t ON t.id = s.tariff_id
		WHERE ($1 = '' OR u.full_name ILIKE '%' || $1 || '%' OR u.login ILIKE '%' || $1 || '%'
		       OR s.msisdn ILIKE '%' || $1 || '%' OR s.account_number ILIKE '%' || $1 || '%')
		  AND ($2 = 'all'
		       OR ($2 = 'active' AND NOT (s.is_blocked OR s.admin_blocked))
		       OR ($2 = 'blocked' AND (s.is_blocked OR s.admin_blocked)))
	),
	paged AS (
		SELECT json_build_object(
			'id', id,
			'user', json_build_object('id', user_id, 'login', login, 'full_name', full_name),
			'number', json_build_object(
				'msisdn', msisdn,
				'account_number', account_number,
				'is_blocked', is_blocked,
				'admin_blocked', admin_blocked
			),
			'balance_byn', balance_byn::text,
			'tariff', json_build_object('id', tariff_id, 'name', tariff_name)
		) AS item
		FROM filtered
		ORDER BY id DESC
		LIMIT $3 OFFSET $4
	)
	SELECT json_build_object(
		'items', COALESCE((SELECT json_agg(item) FROM paged), '[]'::json),
		'page', $5::int,
		'limit', $3::int,
		'total', (SELECT count(*) FROM filtered)
	)`
	return r.ReadJSON(ctx, query, search, status, limit, (page-1)*limit, page)
}

func (r *Repository) ReadAdminSubscriber(ctx context.Context, subscriberID int64) (json.RawMessage, error) {
	const op = op + ".ReadAdminSubscriber()"
	const query = `SELECT json_build_object(
		'id', s.id,
		'user', json_build_object(
			'id', u.id,
			'login', u.login,
			'full_name', u.full_name,
			'email', u.email
		),
		'number', json_build_object(
			'msisdn', s.msisdn,
			'account_number', s.account_number,
			'nickname', s.nickname,
			'is_blocked', s.is_blocked,
			'admin_blocked', s.admin_blocked
		),
		'balance_byn', s.balance_byn::text,
		'tariff', json_build_object(
			'id', t.id,
			'name', t.name,
			'monthly_fee_byn', t.monthly_fee_byn::text
		),
		'connected_services', COALESCE((
			SELECT json_agg(json_build_object(
				'id', v.id,
				'name', v.name,
				'connected_at', ss.connected_at
			) ORDER BY ss.connected_at DESC, v.id ASC)
			FROM telecom.subscriber_services ss
			JOIN telecom.services v ON v.id = ss.service_id
			WHERE ss.subscriber_id = s.id AND ss.disconnected_at IS NULL
		), '[]'::json)
	)
	FROM telecom.subscribers s
	JOIN telecom.users u ON u.id = s.user_id
	JOIN telecom.tariffs t ON t.id = s.tariff_id
	WHERE s.id = $1`
	value, err := r.ReadJSON(ctx, query, subscriberID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Абонент не найден")
	}
	if err != nil {
		return nil, fmt.Errorf("%s | reading subscriber failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) UpdateAdminSubscriberBlock(ctx context.Context, subscriberID int64, adminBlocked bool) (json.RawMessage, error) {
	const op = op + ".UpdateAdminSubscriberBlock()"
	const query = `WITH changed AS (
		UPDATE telecom.subscribers
		SET admin_blocked = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, admin_blocked, (is_blocked OR admin_blocked) AS is_blocked
	)
	SELECT row_to_json(changed) FROM changed`
	value, err := r.UpdateJSON(ctx, query, subscriberID, adminBlocked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Абонент не найден")
	}
	if err != nil {
		return nil, fmt.Errorf("%s | updating admin block failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) ReadAdminTariffs(ctx context.Context, search, status string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, code, name, description,
		       monthly_fee_byn::text AS monthly_fee_byn,
		       included_internet_mb, is_featured, is_active
		FROM telecom.tariffs
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR code ILIKE '%' || $1 || '%')
		  AND ($2 = 'all' OR ($2 = 'active' AND is_active) OR ($2 = 'inactive' AND NOT is_active))
	)
	SELECT json_build_object(
		'items', COALESCE(json_agg(row_to_json(items) ORDER BY id ASC), '[]'::json),
		'count', count(*)
	) FROM items`
	return r.ReadJSON(ctx, query, search, status)
}

func (r *Repository) CreateAdminTariff(ctx context.Context, in AdminTariffInput) (json.RawMessage, error) {
	const op = op + ".CreateAdminTariff()"
	const query = `WITH created AS (
		INSERT INTO telecom.tariffs (code, name, description, monthly_fee_byn, included_internet_mb, is_featured, is_active)
		VALUES ($1, $2, $3, $4::numeric, $5, $6, $7)
		RETURNING id, code, name, description, monthly_fee_byn::text AS monthly_fee_byn,
		          included_internet_mb, is_featured, is_active
	)
	SELECT row_to_json(created) FROM created`
	value, err := r.UpdateJSON(ctx, query, in.Code, in.Name, in.Description, in.MonthlyFeeBYN, in.IncludedInternetMB, in.IsFeatured, in.IsActive)
	if err != nil {
		return nil, uniqueOrError(op, err)
	}
	return value, nil
}

func (r *Repository) UpdateAdminTariff(ctx context.Context, id int64, p AdminTariffPatch) (json.RawMessage, error) {
	const op = op + ".UpdateAdminTariff()"
	const query = `WITH changed AS (
		UPDATE telecom.tariffs
		SET code = COALESCE($2::text, code),
		    name = COALESCE($3::text, name),
		    description = COALESCE($4::text, description),
		    monthly_fee_byn = COALESCE($5::numeric, monthly_fee_byn),
		    included_internet_mb = COALESCE($6::int, included_internet_mb),
		    is_featured = COALESCE($7::boolean, is_featured),
		    is_active = COALESCE($8::boolean, is_active)
		WHERE id = $1
		RETURNING id, code, name, description, monthly_fee_byn::text AS monthly_fee_byn,
		          included_internet_mb, is_featured, is_active
	)
	SELECT row_to_json(changed) FROM changed`
	value, err := r.UpdateJSON(ctx, query, id, p.Code, p.Name, p.Description, p.MonthlyFeeBYN, p.IncludedInternetMB, p.IsFeatured, p.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Тариф не найден")
	}
	if err != nil {
		return nil, uniqueOrError(op, err)
	}
	return value, nil
}

func (r *Repository) DeleteAdminTariff(ctx context.Context, id int64) error {
	const op = op + ".DeleteAdminTariff()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telecom.tariffs WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("%s | tariff lookup failed: %w", op, err)
	}
	if !exists {
		return object.NewError(404, "NOT_FOUND", "Тариф не найден")
	}

	var inUse bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telecom.subscribers WHERE tariff_id = $1)`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("%s | tariff usage check failed: %w", op, err)
	}
	if inUse {
		return object.NewError(409, "TARIFF_IN_USE", "Тариф назначен абонентам и не может быть удалён")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM telecom.tariffs WHERE id = $1`, id); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == "23503" {
			return object.NewError(409, "TARIFF_IN_USE", "Тариф назначен абонентам и не может быть удалён")
		}
		return fmt.Errorf("%s | deleting tariff failed: %w", op, err)
	}
	return tx.Commit()
}

func (r *Repository) ReadAdminServices(ctx context.Context, search, category, status string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, code, name, description, category,
		       monthly_fee_byn::text AS monthly_fee_byn,
		       quota_metric, quota_amount, is_unlimited, is_featured, is_active
		FROM telecom.services
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%' OR code ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR category = $2)
		  AND ($3 = 'all' OR ($3 = 'active' AND is_active) OR ($3 = 'inactive' AND NOT is_active))
	)
	SELECT json_build_object(
		'items', COALESCE(json_agg(row_to_json(items) ORDER BY id ASC), '[]'::json),
		'count', count(*)
	) FROM items`
	return r.ReadJSON(ctx, query, search, category, status)
}

func (r *Repository) ReadAdminServiceByID(ctx context.Context, id int64) (AdminServiceRow, error) {
	const op = op + ".ReadAdminServiceByID()"
	var row AdminServiceRow
	var metric sql.NullString
	var amount sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id, code, name, description, category,
		monthly_fee_byn::text, quota_metric, quota_amount, is_unlimited, is_featured, is_active
		FROM telecom.services WHERE id = $1`, id).
		Scan(&row.ID, &row.Code, &row.Name, &row.Description, &row.Category,
			&row.MonthlyFeeBYN, &metric, &amount, &row.IsUnlimited, &row.IsFeatured, &row.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return row, object.NewError(404, "NOT_FOUND", "Услуга не найдена")
	}
	if err != nil {
		return row, fmt.Errorf("%s | reading service failed: %w", op, err)
	}
	if metric.Valid {
		row.QuotaMetric = &metric.String
	}
	if amount.Valid {
		v := int(amount.Int64)
		row.QuotaAmount = &v
	}
	return row, nil
}

func (r *Repository) CreateAdminService(ctx context.Context, in AdminServiceRow) (json.RawMessage, error) {
	const op = op + ".CreateAdminService()"
	const query = `WITH created AS (
		INSERT INTO telecom.services (code, name, description, category, monthly_fee_byn,
		                              quota_metric, quota_amount, is_unlimited, is_featured, is_active)
		VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10)
		RETURNING id, code, name, description, category, monthly_fee_byn::text AS monthly_fee_byn,
		          quota_metric, quota_amount, is_unlimited, is_featured, is_active
	)
	SELECT row_to_json(created) FROM created`
	value, err := r.UpdateJSON(ctx, query, in.Code, in.Name, in.Description, in.Category,
		in.MonthlyFeeBYN, in.QuotaMetric, in.QuotaAmount, in.IsUnlimited, in.IsFeatured, in.IsActive)
	if err != nil {
		return nil, uniqueOrError(op, err)
	}
	return value, nil
}

func (r *Repository) UpdateAdminService(ctx context.Context, in AdminServiceRow) (json.RawMessage, error) {
	const op = op + ".UpdateAdminService()"
	const query = `WITH changed AS (
		UPDATE telecom.services
		SET code = $2,
		    name = $3,
		    description = $4,
		    category = $5,
		    monthly_fee_byn = $6::numeric,
		    quota_metric = $7,
		    quota_amount = $8,
		    is_unlimited = $9,
		    is_featured = $10,
		    is_active = $11
		WHERE id = $1
		RETURNING id, code, name, description, category, monthly_fee_byn::text AS monthly_fee_byn,
		          quota_metric, quota_amount, is_unlimited, is_featured, is_active
	)
	SELECT row_to_json(changed) FROM changed`
	value, err := r.UpdateJSON(ctx, query, in.ID, in.Code, in.Name, in.Description, in.Category,
		in.MonthlyFeeBYN, in.QuotaMetric, in.QuotaAmount, in.IsUnlimited, in.IsFeatured, in.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Услуга не найдена")
	}
	if err != nil {
		return nil, uniqueOrError(op, err)
	}
	return value, nil
}

func (r *Repository) DeleteAdminService(ctx context.Context, id int64) error {
	const op = op + ".DeleteAdminService()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telecom.services WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("%s | service lookup failed: %w", op, err)
	}
	if !exists {
		return object.NewError(404, "NOT_FOUND", "Услуга не найдена")
	}

	var inUse bool
	if err := tx.QueryRowContext(ctx, `SELECT (
		EXISTS(SELECT 1 FROM telecom.subscriber_services WHERE service_id = $1) OR
		EXISTS(SELECT 1 FROM telecom.usage_events WHERE service_id = $1)
	)`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("%s | service usage check failed: %w", op, err)
	}
	if inUse {
		return object.NewError(409, "SERVICE_IN_USE", "Услуга использовалась абонентами и не может быть удалена")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM telecom.services WHERE id = $1`, id); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == "23503" {
			return object.NewError(409, "SERVICE_IN_USE", "Услуга использовалась абонентами и не может быть удалена")
		}
		return fmt.Errorf("%s | deleting service failed: %w", op, err)
	}
	return tx.Commit()
}
