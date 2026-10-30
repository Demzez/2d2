package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"telecom/internal/object"
)

func (r *Repository) ReadFinanceSummary(ctx context.Context, id int64, month string) (json.RawMessage, error) {
	const query = `SELECT json_build_object(
		'balance_byn', s.balance_byn::text, 'current_month', $2::text,
		'current_month_charges_byn', (SELECT coalesce(sum(amount_byn + vat_byn), 0)::numeric(12,2)::text
		FROM telecom.charges WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date),
		'outstanding_invoices_byn', (SELECT coalesce(sum(total_amount_byn - paid_amount_byn),0)::numeric(12,2)::text
		FROM telecom.invoices WHERE subscriber_id = $1),
		'latest_invoice', (SELECT json_build_object('id', i.id, 'period', to_char(i.period_start, 'YYYY-MM'),
		'total_byn', i.total_amount_byn::text) FROM telecom.invoices i WHERE i.subscriber_id = $1
		ORDER BY i.period_start DESC LIMIT 1)) FROM telecom.subscribers s WHERE s.id = $1`
	return r.ReadJSON(ctx, query, id, month)
}

func (r *Repository) ReadCharges(ctx context.Context, id int64, month string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, charge_type AS type, description,
		amount_byn::text AS amount_byn, vat_byn::text AS vat_byn,
		(amount_byn + vat_byn)::numeric(12,2)::text AS total_byn, charged_at
		FROM telecom.charges WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date
		ORDER BY charged_at DESC, id DESC)
	SELECT json_build_object('month', $2::text,
		'total_byn', (SELECT coalesce(sum(amount_byn + vat_byn),0)::numeric(12,2)::text FROM telecom.charges
		WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date),
		'items', coalesce(json_agg(row_to_json(items)), '[]'::json)) FROM items`
	return r.ReadJSON(ctx, query, id, month)
}

func (r *Repository) ReadInvoices(ctx context.Context, id int64, months int, currentMonth string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, to_char(period_start, 'YYYY-MM') AS period, total_amount_byn::text AS total_byn,
		paid_amount_byn::text AS paid_byn, (total_amount_byn - paid_amount_byn)::numeric(12,2)::text AS due_byn,
		CASE WHEN paid_amount_byn >= total_amount_byn THEN 'paid' WHEN paid_amount_byn > 0
		THEN 'partially_paid' ELSE 'unpaid' END AS status, issued_at
		FROM telecom.invoices WHERE subscriber_id = $1 AND period_start >= (($2 || '-01')::date - ($3::int * interval '1 month'))
		AND period_start < ($2 || '-01')::date ORDER BY period_start DESC)
	SELECT json_build_object('items', coalesce(json_agg(row_to_json(items)), '[]'::json)) FROM items`
	return r.ReadJSON(ctx, query, id, currentMonth, months)
}

func (r *Repository) ReadInvoice(ctx context.Context, subscriberID, invoiceID int64) (json.RawMessage, error) {
	const op = op + ".ReadInvoice()"
	const query = `SELECT json_build_object('id', i.id, 'period', to_char(i.period_start, 'YYYY-MM'),
		'total_byn', i.total_amount_byn::text, 'paid_byn', i.paid_amount_byn::text,
		'due_byn', (i.total_amount_byn - i.paid_amount_byn)::numeric(12,2)::text,
		'status', CASE WHEN i.paid_amount_byn >= i.total_amount_byn THEN 'paid'
		WHEN i.paid_amount_byn > 0 THEN 'partially_paid' ELSE 'unpaid' END,
		'items', (SELECT coalesce(json_agg(json_build_object('type', c.charge_type, 'description', c.description,
		'amount_byn', c.amount_byn::text, 'vat_byn', c.vat_byn::text,
		'total_byn', (c.amount_byn+c.vat_byn)::numeric(12,2)::text) ORDER BY c.id), '[]'::json)
		FROM telecom.charges c WHERE c.invoice_id = i.id AND c.subscriber_id = $1))
		FROM telecom.invoices i WHERE i.id = $2 AND i.subscriber_id = $1`
	value, err := r.ReadJSON(ctx, query, subscriberID, invoiceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, object.NewError(404, "NOT_FOUND", "Счёт не найден")
	}
	if err != nil {
		return nil, fmt.Errorf("%s | invoice lookup failed: %w", op, err)
	}
	return value, nil
}

func (r *Repository) ReadPayments(ctx context.Context, id int64, months int, month string) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, amount_byn::text AS amount_byn, status, created_at, confirmed_at
		FROM telecom.payments WHERE subscriber_id = $1
		AND created_at >= ((($2 || '-01')::date - (($3::int - 1) * interval '1 month')) AT TIME ZONE 'Europe/Minsk')
		ORDER BY created_at DESC, id DESC)
	SELECT json_build_object('items', coalesce(json_agg(row_to_json(items)), '[]'::json)) FROM items`
	return r.ReadJSON(ctx, query, id, month, months)
}

func (r *Repository) ReadUsageDetails(ctx context.Context, id int64, from, to string, page, limit int) (json.RawMessage, error) {
	const query = `WITH items AS (
		SELECT id, occurred_at, metric, quantity::float8 AS quantity,
		CASE WHEN metric LIKE '%_mb' THEN 'MB' ELSE 'min' END AS unit,
		peer_number, service_id, cost_byn::text AS cost_byn
		FROM telecom.usage_events WHERE subscriber_id = $1
		AND occurred_at >= ($2::date AT TIME ZONE 'Europe/Minsk')
		AND occurred_at < (($3::date + interval '1 day') AT TIME ZONE 'Europe/Minsk')
		ORDER BY occurred_at DESC, id DESC LIMIT $4 OFFSET $5)
	SELECT json_build_object('items', (SELECT coalesce(json_agg(row_to_json(items)), '[]'::json) FROM items),
		'page', $6::int, 'limit', $4::int,
		'total', (SELECT count(*) FROM telecom.usage_events WHERE subscriber_id = $1
		AND occurred_at >= ($2::date AT TIME ZONE 'Europe/Minsk')
		AND occurred_at < (($3::date + interval '1 day') AT TIME ZONE 'Europe/Minsk')))`
	return r.ReadJSON(ctx, query, id, from, to, limit, (page-1)*limit, page)
}

func (r *Repository) ReadAnalytics(ctx context.Context, id int64, month string, months int) (json.RawMessage, error) {
	const query = `WITH monthly AS (
		SELECT d::date AS date, to_char(d, 'YYYY-MM') AS month FROM generate_series(
		(($2 || '-01')::date - (($3::int - 1) * interval '1 month')),
		($2 || '-01')::date, interval '1 month') AS d
	), minutes AS (
		SELECT m.month, coalesce(sum(e.quantity) FILTER (WHERE e.metric = 'minutes_own'),0)::float8 AS own,
		coalesce(sum(e.quantity) FILTER (WHERE e.metric = 'minutes_other'),0)::float8 AS other
		FROM monthly m LEFT JOIN telecom.usage_events e ON e.subscriber_id = $1
		AND e.occurred_at >= (m.date AT TIME ZONE 'Europe/Minsk')
		AND e.occurred_at < ((m.date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		AND e.metric IN ('minutes_own', 'minutes_other') GROUP BY m.month, m.date ORDER BY m.date
	), charges AS (
		SELECT charge_type AS type, sum(amount_byn + vat_byn)::numeric(12,2) AS amount
		FROM telecom.charges WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date GROUP BY charge_type
	), charge_total AS (SELECT coalesce(sum(amount),0)::numeric(12,2) AS amount FROM charges),
	popularity AS (
		SELECT v.id AS service_id, v.name,
		(coalesce(count(DISTINCT ss.subscriber_id),0)::float8 * 100.0 / nullif((SELECT count(*) FROM telecom.subscribers),0))::numeric(7,2)::float8 AS percent_connected
		FROM telecom.services v LEFT JOIN telecom.subscriber_services ss ON ss.service_id = v.id AND ss.disconnected_at IS NULL
		WHERE v.is_active GROUP BY v.id, v.name ORDER BY percent_connected DESC NULLS LAST, v.id LIMIT 3
	), days AS (
		SELECT d::date AS day FROM generate_series(($2 || '-01')::date,
		(($2 || '-01')::date + interval '1 month' - interval '1 day'), interval '1 day') AS d
	), daily AS (
		SELECT to_char(d.day, 'YYYY-MM-DD') AS date,
		coalesce(sum(e.quantity),0)::float8 AS used_mb FROM days d LEFT JOIN telecom.usage_events e
		ON e.subscriber_id = $1 AND e.metric IN ('internet_mb', 'messenger_mb')
		AND e.occurred_at >= (d.day AT TIME ZONE 'Europe/Minsk')
		AND e.occurred_at < ((d.day + interval '1 day') AT TIME ZONE 'Europe/Minsk')
		GROUP BY d.day ORDER BY d.day
	)
	SELECT json_build_object(
		'selected_month', $2::text, 'months_count', $3::int,
		'minutes_by_month', (SELECT coalesce(json_agg(row_to_json(minutes)), '[]'::json) FROM minutes),
		'payment_distribution', json_build_object(
		'total_byn', (SELECT amount::text FROM charge_total),
		'parts', (SELECT coalesce(json_agg(json_build_object('type', c.type,
		'amount_byn', c.amount::text,
		'percent', CASE WHEN ct.amount = 0 THEN 0 ELSE round(c.amount * 100 / ct.amount, 2) END)::json ORDER BY c.type), '[]'::json)
		FROM charges c CROSS JOIN charge_total ct)),
		'popular_services', (SELECT coalesce(json_agg(row_to_json(popularity)), '[]'::json) FROM popularity),
		'internet_by_day', (SELECT coalesce(json_agg(row_to_json(daily)), '[]'::json) FROM daily),
		'calendar_activity_days', (SELECT coalesce(json_agg(d ORDER BY d), '[]'::json) FROM (
		SELECT DISTINCT to_char(occurred_at AT TIME ZONE 'Europe/Minsk', 'YYYY-MM-DD') AS d
		FROM telecom.usage_events WHERE subscriber_id = $1
		AND occurred_at >= (($2 || '-01')::date AT TIME ZONE 'Europe/Minsk')
		AND occurred_at < ((($2 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')) activity),
		'unread_alerts', (SELECT count(*) FROM telecom.notifications
		WHERE subscriber_id = $1 AND kind = 'alert' AND read_at IS NULL))`
	return r.ReadJSON(ctx, query, id, month, months)
}

// Money is converted to integer kopecks before any payment-side arithmetic.
func MoneyToKopecks(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != 2 || len(parts[0]) == 0 || len(parts[0]) > 10 {
		return 0, object.NewError(400, "VALIDATION_ERROR", "Сумма должна иметь два знака после точки")
	}
	for _, digit := range parts[0] + parts[1] {
		if digit < '0' || digit > '9' {
			return 0, object.NewError(400, "VALIDATION_ERROR", "Неверная сумма")
		}
	}
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	fraction, _ := strconv.ParseInt(parts[1], 10, 64)
	if whole > (1<<62)/100 {
		return 0, object.NewError(400, "VALIDATION_ERROR", "Сумма слишком велика")
	}
	return whole*100 + fraction, nil
}

func (r *Repository) CreatePayment(ctx context.Context, subscriberID int64, key, amount string) (json.RawMessage, error) {
	const op = op + ".CreatePayment()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()
	// Single subscriber lock makes repeated requests with one key deterministic.
	var lockedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM telecom.subscribers WHERE id = $1 FOR UPDATE`, subscriberID).Scan(&lockedID); err != nil {
		return nil, fmt.Errorf("%s | locking subscriber failed: %w", op, err)
	}
	var id int64
	var savedAmount string
	var status string
	err = tx.QueryRowContext(ctx, `SELECT id, amount_byn::text, status FROM telecom.payments
		WHERE subscriber_id = $1 AND idempotency_key = $2`, subscriberID, key).Scan(&id, &savedAmount, &status)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `INSERT INTO telecom.payments(subscriber_id, idempotency_key, amount_byn)
		VALUES ($1,$2,$3::numeric) RETURNING id, amount_byn::text, status`, subscriberID, key, amount).Scan(&id, &savedAmount, &status)
		if err != nil {
			return nil, fmt.Errorf("%s | creating payment failed: %w", op, err)
		}
		_, err = tx.ExecContext(ctx, `UPDATE telecom.payments SET provider_payment_id = $2 WHERE id = $1`, id, fmt.Sprintf("mock_%d", id))
		if err != nil {
			return nil, fmt.Errorf("%s | setting provider reference failed: %w", op, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("%s | reading payment failed: %w", op, err)
	}
	if savedAmount != amount {
		return nil, object.NewError(409, "CONFLICT", "Idempotency-Key уже использован с другой суммой")
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s | commit failed: %w", op, err)
	}
	data, _ := json.Marshal(map[string]any{"payment_id": id, "status": status, "checkout_url": nil})
	return data, nil
}

type PaymentEvent struct {
	EventID           string `json:"event_id"`
	ProviderPaymentID string `json:"provider_payment_id"`
	Status            string `json:"status"`
	Amount            string `json:"amount_byn"`
	Currency          string `json:"currency"`
}

func (r *Repository) UpdatePayment(ctx context.Context, event PaymentEvent) error {
	const op = op + ".UpdatePayment()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()
	// Protect the invoice set against concurrent topups for the same account.
	var subID, payID int64
	var amount string
	var status string
	err = tx.QueryRowContext(ctx, `SELECT subscriber_id, id FROM telecom.payments WHERE provider_payment_id = $1`, event.ProviderPaymentID).Scan(&subID, &payID)
	if errors.Is(err, sql.ErrNoRows) {
		return object.NewError(404, "NOT_FOUND", "Платёж не найден")
	}
	if err != nil {
		return fmt.Errorf("%s | locating payment failed: %w", op, err)
	}
	var balance string
	if err := tx.QueryRowContext(ctx, `SELECT balance_byn::text FROM telecom.subscribers WHERE id = $1 FOR UPDATE`, subID).Scan(&balance); err != nil {
		return fmt.Errorf("%s | locking subscriber failed: %w", op, err)
	}
	err = tx.QueryRowContext(ctx, `SELECT amount_byn::text, status FROM telecom.payments WHERE id = $1 FOR UPDATE`, payID).Scan(&amount, &status)
	if err != nil {
		return fmt.Errorf("%s | locking payment failed: %w", op, err)
	}
	if amount != event.Amount {
		return object.NewError(409, "CONFLICT", "Сумма платежа не совпадает")
	}
	if status == event.Status {
		return nil
	}
	if status != "pending" {
		return object.NewError(409, "CONFLICT", "Платёж уже завершён")
	}
	if event.Status == "failed" {
		_, err = tx.ExecContext(ctx, `UPDATE telecom.payments SET status = 'failed' WHERE id = $1`, payID)
		if err != nil {
			return fmt.Errorf("%s | failing payment failed: %w", op, err)
		}
		return tx.Commit()
	}
	amountLeft, err := MoneyToKopecks(amount)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, (total_amount_byn - paid_amount_byn)::text
		FROM telecom.invoices WHERE subscriber_id = $1 AND total_amount_byn > paid_amount_byn
		ORDER BY period_start ASC FOR UPDATE`, subID)
	if err != nil {
		return fmt.Errorf("%s | loading unpaid invoices failed: %w", op, err)
	}
	type debt struct{ id, kopecks int64 }
	var debts []debt
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			rows.Close()
			return fmt.Errorf("%s | scanning debt failed: %w", op, err)
		}
		value, err := MoneyToKopecks(text)
		if err != nil {
			rows.Close()
			return err
		}
		debts = append(debts, debt{id, value})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("%s | reading invoices failed: %w", op, err)
	}
	rows.Close()
	for _, debt := range debts {
		if amountLeft == 0 {
			break
		}
		paid := debt.kopecks
		if paid > amountLeft {
			paid = amountLeft
		}
		_, err = tx.ExecContext(ctx, `UPDATE telecom.invoices SET paid_amount_byn = paid_amount_byn + $2::numeric WHERE id = $1`, debt.id, fmt.Sprintf("%d.%02d", paid/100, paid%100))
		if err != nil {
			return fmt.Errorf("%s | paying invoice failed: %w", op, err)
		}
		amountLeft -= paid
	}
	_, err = tx.ExecContext(ctx, `UPDATE telecom.subscribers SET balance_byn = balance_byn + $2::numeric, updated_at = now() WHERE id = $1`, subID, amount)
	if err != nil {
		return fmt.Errorf("%s | updating balance failed: %w", op, err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE telecom.payments SET status = 'succeeded', confirmed_at = now() WHERE id = $1`, payID)
	if err != nil {
		return fmt.Errorf("%s | confirming payment failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return nil
}

func (r *Repository) RunBilling(ctx context.Context, month string) error {
	const op = op + ".RunBilling()"
	// Periodic invocation is idempotent for a tariff using the unique (subscriber, period, kind) insert filter.
	// Existing service attachments are charged only when first creating monthly charges.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(424242)`) // Single billing execution.
	if err != nil {
		return fmt.Errorf("%s | billing lock failed: %w", op, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO telecom.charges(subscriber_id, period_start, charge_type, description, amount_byn, vat_byn)
		SELECT s.id, ($1 || '-01')::date, 'tariff', t.name, t.monthly_fee_byn,
		round(t.monthly_fee_byn * 0.20, 2) FROM telecom.subscribers s JOIN telecom.tariffs t ON t.id = s.tariff_id
		WHERE s.created_at < ((($1 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		AND NOT EXISTS (SELECT 1 FROM telecom.charges c WHERE c.subscriber_id = s.id
		AND c.period_start = ($1 || '-01')::date AND c.charge_type = 'tariff')`, month)
	if err != nil {
		return fmt.Errorf("%s | tariff charges failed: %w", op, err)
	}
	// No simulated usage or service activation is inserted.
	_, err = tx.ExecContext(ctx, `INSERT INTO telecom.charges(subscriber_id, period_start, charge_type, description, amount_byn, vat_byn)
		SELECT s.id, ($1 || '-01')::date, 'service', v.name, v.monthly_fee_byn,
		round(v.monthly_fee_byn * 0.20,2)
		FROM telecom.subscribers s JOIN telecom.subscriber_services ss ON ss.subscriber_id = s.id
		JOIN telecom.services v ON v.id = ss.service_id
		WHERE v.monthly_fee_byn > 0
		AND s.created_at < ((($1 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		AND ss.connected_at < ((($1 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		AND (ss.disconnected_at IS NULL OR ss.disconnected_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk'))
		AND NOT EXISTS (SELECT 1 FROM telecom.charges c WHERE c.subscriber_id = s.id
		AND c.period_start = ($1 || '-01')::date AND c.charge_type = 'service' AND c.description = v.name)`, month)
	if err != nil {
		return fmt.Errorf("%s | service charges failed: %w", op, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s | billing commit failed: %w", op, err)
	}
	return nil
}

func (r *Repository) CloseBillingMonth(ctx context.Context, month string) error {
	const op = op + ".CloseBillingMonth()"
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s | transaction failed: %w", op, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(424242)`); err != nil {
		return fmt.Errorf("%s | billing lock failed: %w", op, err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.id FROM telecom.subscribers s
		WHERE EXISTS (SELECT 1 FROM telecom.charges c WHERE c.subscriber_id = s.id AND c.period_start = ($1 || '-01')::date)
		AND NOT EXISTS (SELECT 1 FROM telecom.invoices i WHERE i.subscriber_id = s.id AND i.period_start = ($1 || '-01')::date)
		ORDER BY s.id FOR UPDATE`, month)
	if err != nil {
		return fmt.Errorf("%s | loading accounts for closure failed: %w", op, err)
	}
	var subscriberIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		subscriberIDs = append(subscriberIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range subscriberIDs {
		var total string
		var balance string
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(sum(amount_byn + vat_byn),0)::numeric(12,2)::text FROM telecom.charges WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date`, id, month).Scan(&total); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT balance_byn::text FROM telecom.subscribers WHERE id = $1`, id).Scan(&balance); err != nil {
			return err
		}
		paid := int64(0)
		totalCents, err := MoneyToKopecks(total)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(balance, "-") {
			balanceCents, err := MoneyToKopecks(balance)
			if err != nil {
				return err
			}
			paid = balanceCents
			if paid > totalCents {
				paid = totalCents
			}
		}
		var invoiceID int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO telecom.invoices(subscriber_id, period_start, total_amount_byn, paid_amount_byn)
		VALUES ($1, ($2 || '-01')::date, $3::numeric, $4::numeric) RETURNING id`, id, month,
			total, fmt.Sprintf("%d.%02d", paid/100, paid%100)).Scan(&invoiceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE telecom.charges SET invoice_id = $3 WHERE subscriber_id = $1 AND period_start = ($2 || '-01')::date`, id, month, invoiceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE telecom.subscribers SET balance_byn = balance_byn - $2::numeric, updated_at = now() WHERE id = $1`, id, total); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s | commit failed: %w", op, err)
	}
	return nil
}

func (r *Repository) CreateAlerts(ctx context.Context, month string) error {
	const op = op + ".CreateAlerts()"
	const negativeBalance = `INSERT INTO telecom.notifications(subscriber_id, kind, title, body, target_path)
		SELECT s.id, 'alert', 'Отрицательный баланс', 'Баланс вашего лицевого счёта отрицательный', '/finance'
		FROM telecom.subscribers s WHERE s.balance_byn < 0 AND NOT EXISTS (
		SELECT 1 FROM telecom.notifications n WHERE n.subscriber_id = s.id
		AND n.title = 'Отрицательный баланс'
		AND n.created_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk'))`
	if _, err := r.db.ExecContext(ctx, negativeBalance, month); err != nil {
		return fmt.Errorf("%s | negative balance alerts failed: %w", op, err)
	}

	const lowInternet = `WITH usage AS (
		SELECT subscriber_id, sum(quantity) AS consumed FROM telecom.usage_events
		WHERE metric = 'internet_mb' AND service_id IS NULL
		AND occurred_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk')
		AND occurred_at < ((($1 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		GROUP BY subscriber_id
	)
	INSERT INTO telecom.notifications(subscriber_id, kind, title, body, target_path)
	SELECT s.id, 'alert', 'Заканчивается пакет интернета',
		'Использовано не менее 80% включённого интернет-трафика', '/services'
	FROM telecom.subscribers s JOIN telecom.tariffs t ON t.id = s.tariff_id
	JOIN usage u ON u.subscriber_id = s.id
	WHERE t.included_internet_mb > 0 AND u.consumed >= t.included_internet_mb * 0.8
	AND NOT EXISTS (SELECT 1 FROM telecom.notifications n WHERE n.subscriber_id = s.id
	AND n.title = 'Заканчивается пакет интернета'
	AND n.created_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk'))`
	if _, err := r.db.ExecContext(ctx, lowInternet, month); err != nil {
		return fmt.Errorf("%s | internet usage alerts failed: %w", op, err)
	}

	const lowServices = `WITH usage AS (
		SELECT ss.subscriber_id, v.id AS service_id, v.name, v.quota_amount,
		coalesce(sum(e.quantity),0) AS consumed
		FROM telecom.subscriber_services ss JOIN telecom.services v ON v.id = ss.service_id
		LEFT JOIN telecom.usage_events e ON e.subscriber_id = ss.subscriber_id
		AND e.service_id = v.id AND e.metric = v.quota_metric
		AND e.occurred_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk')
		AND e.occurred_at < ((($1 || '-01')::date + interval '1 month') AT TIME ZONE 'Europe/Minsk')
		WHERE ss.disconnected_at IS NULL AND v.quota_amount > 0
		GROUP BY ss.subscriber_id, v.id, v.name, v.quota_amount
	)
	INSERT INTO telecom.notifications(subscriber_id, kind, title, body, target_path)
	SELECT u.subscriber_id, 'alert', 'Заканчивается пакет: ' || left(u.name, 120),
		'Использовано не менее 80% пакета услуги', '/services'
	FROM usage u WHERE u.consumed >= u.quota_amount * 0.8
	AND NOT EXISTS (SELECT 1 FROM telecom.notifications n WHERE n.subscriber_id = u.subscriber_id
	AND n.title = 'Заканчивается пакет: ' || left(u.name, 120)
	AND n.created_at >= (($1 || '-01')::date AT TIME ZONE 'Europe/Minsk'))`
	if _, err := r.db.ExecContext(ctx, lowServices, month); err != nil {
		return fmt.Errorf("%s | service usage alerts failed: %w", op, err)
	}
	return nil
}
