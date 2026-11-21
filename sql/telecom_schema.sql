-- 2D2: схема PostgreSQL 15+ для кабинета абонента и администратора.
-- Запуск: psql -d telecom -v ON_ERROR_STOP=1 -f telecom_schema.sql
-- Все суммы в BYN, трафик в МБ, звонки в минутах.
-- TIMESTAMPTZ хранится как абсолютное время; месяцы считаются по Europe/Minsk.

BEGIN;

CREATE SCHEMA IF NOT EXISTS telecom;
SET search_path TO telecom, public;

-- 1. Тарифы (каталог).
CREATE TABLE IF NOT EXISTS tariffs (
                                       id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                       code                   VARCHAR(50) NOT NULL UNIQUE,
                                       name                   VARCHAR(120) NOT NULL,
                                       description            TEXT NOT NULL DEFAULT '',
                                       monthly_fee_byn        NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (monthly_fee_byn >= 0),
                                       included_internet_mb   INTEGER NOT NULL DEFAULT 0 CHECK (included_internet_mb >= 0),
                                       is_featured            BOOLEAN NOT NULL DEFAULT FALSE,
                                       is_active              BOOLEAN NOT NULL DEFAULT TRUE
);

-- 2. Пользователи, авторизация по логину и паролю.
CREATE TABLE IF NOT EXISTS users (
                                     id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                     login               VARCHAR(50) NOT NULL,
                                     password_hash       TEXT NOT NULL,                 -- Argon2id; никогда не хранить пароль
                                     role                VARCHAR(12) NOT NULL DEFAULT 'user',
                                     full_name           VARCHAR(150) NOT NULL,
                                     email               VARCHAR(254),
                                     privacy_accepted_at TIMESTAMPTZ NOT NULL,
                                     created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
                                     updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
                                     CONSTRAINT users_login_length CHECK (char_length(login) BETWEEN 3 AND 50),
                                     CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'))
);

-- Миграция для уже существующей БД: добавляет роль без потери пользователей.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role VARCHAR(12) NOT NULL DEFAULT 'user';

DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'users_role_check'
              AND conrelid = 'telecom.users'::regclass
        ) THEN
            ALTER TABLE users
                ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));
        END IF;
    END $$;

COMMENT ON COLUMN users.role IS
    'Роль учетной записи: user — абонент, admin — администратор.';

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_login_ci ON users (lower(login));
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_ci ON users (lower(email)) WHERE email IS NOT NULL;

-- 3. Один мобильный номер / лицевой счёт на пользователя (упрощение учебного проекта).
CREATE TABLE IF NOT EXISTS subscribers (
                                           id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                           user_id         BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
                                           account_number  VARCHAR(40) NOT NULL UNIQUE,
                                           msisdn          VARCHAR(16) NOT NULL UNIQUE,
                                           nickname        VARCHAR(100),
                                           tariff_id       BIGINT NOT NULL REFERENCES tariffs(id),
                                           balance_byn     NUMERIC(12, 2) NOT NULL DEFAULT 0,
                                           is_blocked      BOOLEAN NOT NULL DEFAULT FALSE,
                                           admin_blocked   BOOLEAN NOT NULL DEFAULT FALSE,
                                           created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                           updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                           CONSTRAINT subscribers_msisdn_format CHECK (msisdn ~ '^[+][1-9][0-9]{7,14}$')
);

-- Миграция для уже существующей БД: отдельная блокировка администратора.
ALTER TABLE subscribers
    ADD COLUMN IF NOT EXISTS admin_blocked BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN subscribers.admin_blocked IS
    'Административная блокировка номера; не изменяет личную блокировку is_blocked.';

CREATE INDEX IF NOT EXISTS ix_subscribers_tariff ON subscribers(tariff_id);

-- 4. Одноразовые SMS-коды: храним HMAC кода, а не шестизначный код.
CREATE TABLE IF NOT EXISTS auth_otps (
                                         id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                         phone        VARCHAR(16) NOT NULL,
                                         purpose      VARCHAR(12) NOT NULL CHECK (purpose IN ('login', 'register')),
                                         code_hash    CHAR(64) NOT NULL,
                                         attempts     SMALLINT NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
                                         expires_at   TIMESTAMPTZ NOT NULL,
                                         consumed_at  TIMESTAMPTZ,
                                         created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
                                         CONSTRAINT auth_otps_phone_format CHECK (phone ~ '^[+][1-9][0-9]{7,14}$'),
                                         CONSTRAINT auth_otps_time CHECK (expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS ix_auth_otps_lookup ON auth_otps(phone, purpose, created_at DESC);

-- 5. Сессии refresh-token (в таблице только SHA-256 токена).
CREATE TABLE IF NOT EXISTS auth_sessions (
                                             id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                             user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                             token_hash   CHAR(64) NOT NULL UNIQUE,
                                             expires_at   TIMESTAMPTZ NOT NULL,
                                             revoked_at   TIMESTAMPTZ,
                                             created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
                                             CONSTRAINT auth_sessions_time CHECK (expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS ix_auth_sessions_user ON auth_sessions(user_id);

-- 6. Каталог подключаемых услуг.
CREATE TABLE IF NOT EXISTS services (
                                        id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                        code              VARCHAR(60) NOT NULL UNIQUE,
                                        name              VARCHAR(150) NOT NULL,
                                        description       TEXT NOT NULL DEFAULT '',
                                        category          VARCHAR(20) NOT NULL CHECK
                                            (category IN ('internet', 'messengers', 'calls', 'security', 'roaming', 'other')),
                                        monthly_fee_byn   NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (monthly_fee_byn >= 0),
                                        quota_metric      VARCHAR(20) CHECK
                                            (quota_metric IN ('internet_mb', 'messenger_mb', 'minutes_own',
                                                              'minutes_other', 'minutes_roaming')),
                                        quota_amount      INTEGER CHECK (quota_amount > 0),
                                        is_unlimited      BOOLEAN NOT NULL DEFAULT FALSE,
                                        is_featured       BOOLEAN NOT NULL DEFAULT FALSE,
                                        is_active         BOOLEAN NOT NULL DEFAULT TRUE,
                                        CONSTRAINT services_quota_consistency CHECK (
                                            (quota_amount IS NULL OR quota_metric IS NOT NULL)
                                                AND (NOT is_unlimited OR (quota_metric IS NOT NULL AND quota_amount IS NULL))
                                            )
);
CREATE INDEX IF NOT EXISTS ix_services_category ON services(category, is_active);

-- 7. История подключений и отключений. NULL = подключено сейчас.
CREATE TABLE IF NOT EXISTS subscriber_services (
                                                   id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                                   subscriber_id   BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                                   service_id      BIGINT NOT NULL REFERENCES services(id),
                                                   connected_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
                                                   disconnected_at TIMESTAMPTZ,
                                                   CONSTRAINT subscriber_services_dates CHECK (
                                                       disconnected_at IS NULL OR disconnected_at >= connected_at
                                                       )
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_subscriber_active_service
    ON subscriber_services(subscriber_id, service_id)
    WHERE disconnected_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_subscriber_services_history
    ON subscriber_services(subscriber_id, connected_at DESC);

-- 8. Фактическое потребление. Для *_mb значение = МБ, для minutes_* = минуты.
-- Записи поступают из биллинга/эмулятора, абонент не может их создавать через API.
CREATE TABLE IF NOT EXISTS usage_events (
                                            id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                            subscriber_id   BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                            service_id      BIGINT REFERENCES services(id),
                                            metric          VARCHAR(20) NOT NULL CHECK
                                                (metric IN ('internet_mb', 'messenger_mb', 'minutes_own',
                                                            'minutes_other', 'minutes_roaming')),
                                            quantity        NUMERIC(12, 3) NOT NULL CHECK (quantity > 0),
                                            cost_byn        NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (cost_byn >= 0),
                                            peer_number     VARCHAR(16),                 -- номер собеседника для детализации звонков
                                            occurred_at     TIMESTAMPTZ NOT NULL,
                                            created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                            CONSTRAINT usage_events_peer_format CHECK (
                                                peer_number IS NULL OR peer_number ~ '^[+][1-9][0-9]{7,14}$'
                                                )
);
CREATE INDEX IF NOT EXISTS ix_usage_subscriber_time
    ON usage_events(subscriber_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS ix_usage_subscriber_metric_time
    ON usage_events(subscriber_id, metric, occurred_at DESC);

-- 9. Счета за завершённые месяцы.
-- total_amount_byn = SUM(charges.amount_byn + charges.vat_byn) по строкам счёта;
-- paid_amount_byn увеличивает биллинг при списании с баланса/погашении долга.
CREATE TABLE IF NOT EXISTS invoices (
                                        id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                        subscriber_id    BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                        period_start     DATE NOT NULL,
                                        total_amount_byn NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (total_amount_byn >= 0),
                                        paid_amount_byn  NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (paid_amount_byn >= 0),
                                        issued_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
                                        CONSTRAINT invoices_period_first_day CHECK (EXTRACT(DAY FROM period_start) = 1),
                                        CONSTRAINT invoices_paid_not_over CHECK (paid_amount_byn <= total_amount_byn),
                                        CONSTRAINT uq_invoices_subscriber_period UNIQUE (subscriber_id, period_start),
                                        CONSTRAINT uq_invoices_id_subscriber_period UNIQUE (id, subscriber_id, period_start)
);
CREATE INDEX IF NOT EXISTS ix_invoices_subscriber_period
    ON invoices(subscriber_id, period_start DESC);

-- 10. Начисления: до выставления счёта invoice_id = NULL.
CREATE TABLE IF NOT EXISTS charges (
                                       id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                       subscriber_id   BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                       invoice_id      BIGINT,
                                       period_start    DATE NOT NULL,
                                       charge_type     VARCHAR(12) NOT NULL CHECK (charge_type IN ('tariff', 'service', 'usage')),
                                       description     VARCHAR(255) NOT NULL,
                                       amount_byn      NUMERIC(12, 2) NOT NULL CHECK (amount_byn >= 0), -- без НДС
                                       vat_byn         NUMERIC(12, 2) NOT NULL DEFAULT 0 CHECK (vat_byn >= 0),
                                       charged_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                       CONSTRAINT charges_period_first_day CHECK (EXTRACT(DAY FROM period_start) = 1),
                                       CONSTRAINT fk_charges_invoice_correct_owner FOREIGN KEY (invoice_id, subscriber_id, period_start)
                                           REFERENCES invoices(id, subscriber_id, period_start)
);
CREATE INDEX IF NOT EXISTS ix_charges_subscriber_period
    ON charges(subscriber_id, period_start DESC);
CREATE INDEX IF NOT EXISTS ix_charges_invoice ON charges(invoice_id) WHERE invoice_id IS NOT NULL;

-- 11. Пополнения через платёжного провайдера. Зачисление только после проверенного webhook.
CREATE TABLE IF NOT EXISTS payments (
                                        id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                        subscriber_id       BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                        idempotency_key     VARCHAR(100) NOT NULL,
                                        provider_payment_id VARCHAR(120) UNIQUE,
                                        amount_byn          NUMERIC(12, 2) NOT NULL CHECK (amount_byn > 0),
                                        status              VARCHAR(12) NOT NULL DEFAULT 'pending'
                                            CHECK (status IN ('pending', 'succeeded', 'failed')),
                                        created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
                                        confirmed_at        TIMESTAMPTZ,
                                        CONSTRAINT uq_payments_idempotency UNIQUE (subscriber_id, idempotency_key),
                                        CONSTRAINT payments_success_has_time CHECK (status <> 'succeeded' OR confirmed_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS ix_payments_subscriber_time
    ON payments(subscriber_id, created_at DESC);

-- 12. Любимые собеседники.
CREATE TABLE IF NOT EXISTS favorite_contacts (
                                                 id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                                 subscriber_id   BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                                 name            VARCHAR(100) NOT NULL,
                                                 phone           VARCHAR(16) NOT NULL,
                                                 created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                                 CONSTRAINT favorite_contacts_phone_format CHECK (phone ~ '^[+][1-9][0-9]{7,14}$'),
                                                 CONSTRAINT uq_favorite_contacts_number UNIQUE (subscriber_id, phone)
);

-- 13. Уведомления, включая автоматически созданные оповещения об отклонениях.
CREATE TABLE IF NOT EXISTS notifications (
                                             id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
                                             subscriber_id   BIGINT NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE,
                                             kind            VARCHAR(10) NOT NULL CHECK (kind IN ('alert', 'message')),
                                             title           VARCHAR(150) NOT NULL,
                                             body            TEXT NOT NULL,
                                             target_path     VARCHAR(255),              -- только относительный маршрут интерфейса
                                             created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                                             read_at         TIMESTAMPTZ,
                                             CONSTRAINT notifications_local_target CHECK (
                                                 target_path IS NULL OR (target_path LIKE '/%' AND target_path NOT LIKE '//%')
                                                 )
);
CREATE INDEX IF NOT EXISTS ix_notifications_subscriber_time
    ON notifications(subscriber_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_notifications_unread
    ON notifications(subscriber_id, kind) WHERE read_at IS NULL;

COMMIT;
