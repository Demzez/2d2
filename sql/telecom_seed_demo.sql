-- ============================================================================
-- 2D2 / PostgreSQL 15+ / ДЕМОНСТРАЦИОННЫЕ ДАННЫЕ
-- Запуск ПОСЛЕ telecom_schema.sql:
--   psql -d telecom -v ON_ERROR_STOP=1 -f telecom_seed_demo.sql
--
-- Два обычных пользователя и один администратор; данные вымышленные.
-- Время: Europe/Minsk; периоды: два завершённых месяца и текущий месяц.
-- Валюта: BYN; объёмы: МБ; длительность вызовов: минуты.
-- Цены в каталогах — условные цены с НДС 20% для целей демонстрации.
-- В charges.amount_byn сумма БЕЗ НДС, charges.vat_byn — НДС.
-- Разовые события сверх пакета содержат cost_byn С НДС.
--
-- ВАЖНО: Только для локальной разработки / демонстрации, НЕ для production!
-- Сеансов входа и OTP-кодов здесь нет. Платежи — симуляция, не реальные webhook.
-- Argon2id-хеши заранее сгенерированы с уникальной случайной солью.
-- Повторный запуск не создаёт дублей для собственных записей seed-скрипта.
-- ============================================================================

BEGIN;
SET LOCAL search_path TO telecom, public;
SET LOCAL TIME ZONE 'Europe/Minsk';
-- Исключаем параллельные запуски одного и того же наполнения.
SELECT pg_advisory_xact_lock(hashtext('telecom_2d2_demo_seed'));

-- Привязка всех периодов к календарю Минска, независимо от TZ клиента psql.
CREATE TEMP TABLE _seed_clock ON COMMIT DROP AS
SELECT date_trunc('month', now() AT TIME ZONE 'Europe/Minsk') AS month_local,
       (now() AT TIME ZONE 'Europe/Minsk')::date AS today_local;

-- --------------------------------------------------------------------------
-- 1. Каталоги: опорные коды из исходной схемы остаются без изменений.
-- --------------------------------------------------------------------------
INSERT INTO tariffs
(code, name, description, monthly_fee_byn, included_internet_mb,
 is_featured, is_active)
VALUES
    ('comfort-s', 'Комфорт S', '10 ГБ интернета; базовый тариф для повседневной связи',
     12.50, 10240, TRUE, TRUE),
    ('comfort-m', 'Комфорт M', '30 ГБ интернета для активного использования',
     25.90, 30720, TRUE, TRUE),
    ('comfort-l', 'Комфорт L', '60 ГБ мобильного интернета',
     39.90, 61440, TRUE, TRUE),
    ('start-5', 'Старт 5', 'Экономичный тариф с 5 ГБ интернета',
     8.90, 5120, FALSE, TRUE),
    ('archive-2024', 'Архивный 2024', 'Архивный план, не доступный для новых подключений',
     15.50, 10240, FALSE, FALSE)
ON CONFLICT (code) DO NOTHING;

INSERT INTO services
(code, name, description, category, monthly_fee_byn,
 quota_metric, quota_amount, is_unlimited, is_featured, is_active)
VALUES
    ('internet-unlimited-5', 'Безлимитный интернет до 5 Мбит/с',
     'Интернет без ограничения объёма на скорости до 5 Мбит/с',
     'internet', 15.99, 'internet_mb', NULL, TRUE, TRUE, TRUE),
    ('messengers-unlimited', 'Безлимит на мессенджеры',
     'Неограниченный трафик популярных мессенджеров',
     'messengers', 0.00, 'messenger_mb', NULL, TRUE, TRUE, TRUE),
    ('calls-other-60', '60 минут в другие сети',
     '60 минут разговоров с абонентами других сетей в месяц',
     'calls', 0.00, 'minutes_other', 60, FALSE, TRUE, TRUE),
    ('calls-own-unlimited', 'Безлимит минут внутри сети',
     'Неограниченные звонки внутри сети 2D2',
     'calls', 0.00, 'minutes_own', NULL, TRUE, FALSE, TRUE),
    ('safe-internet', 'Безопасный интернет',
     'Дополнительная защита при использовании мобильного интернета',
     'security', 2.90, NULL, NULL, FALSE, FALSE, TRUE),
    ('roaming-calls', 'Звонки в роуминге',
     '30 минут для звонков за пределами домашней сети',
     'roaming', 4.90, 'minutes_roaming', 30, FALSE, TRUE, TRUE),
    ('extra-internet-1gb', 'Дополнительный 1 ГБ',
     'Дополнительный пакет интернета на один месяц',
     'internet', 3.00, 'internet_mb', 1024, FALSE, FALSE, TRUE),
    ('extra-internet-5gb', 'Дополнительные 5 ГБ',
     'Дополнительный месячный пакет из 5 ГБ',
     'internet', 9.90, 'internet_mb', 5120, FALSE, FALSE, TRUE),
    ('roaming-internet-1gb', 'Интернет в роуминге 1 ГБ',
     'Пакет интернета для поездок за пределы домашней сети',
     'roaming', 11.90, 'internet_mb', 1024, FALSE, TRUE, TRUE),
    ('anti-spam', 'Антиспам',
     'Защита от нежелательных вызовов и сообщений',
     'security', 1.90, NULL, NULL, FALSE, FALSE, TRUE),
    ('night-internet', 'Ночной интернет 10 ГБ',
     'Дополнительный пакет трафика в ночное время',
     'internet', 7.50, 'internet_mb', 10240, FALSE, FALSE, TRUE),
    ('archive-info', 'Архивное информирование',
     'Ранее предоставляемая услуга, закрытая для новых подключений',
     'other', 0.00, NULL, NULL, FALSE, FALSE, FALSE)
ON CONFLICT (code) DO NOTHING;

-- --------------------------------------------------------------------------
-- 2. Учётные записи. Argon2id: m=65536,t=2,p=1; соль 16 байт, ключ 32 байта.
--    Для существующих демо-логинов ON CONFLICT обновляет только пароль
--    и updated_at, если совпадают роль и email; другие поля не меняются.
--    Все домены example.com и телефонные номера вымышлены.
-- --------------------------------------------------------------------------
INSERT INTO users
(login, password_hash, role, full_name, email,
 privacy_accepted_at, created_at, updated_at)
SELECT v.login, v.password_hash, v.role, v.full_name, v.email,
       (c.month_local - INTERVAL '2 months' + INTERVAL '7 hours')
           AT TIME ZONE 'Europe/Minsk',
       (c.month_local - INTERVAL '2 months' + INTERVAL '8 hours')
           AT TIME ZONE 'Europe/Minsk',
       (c.month_local - INTERVAL '2 months' + INTERVAL '8 hours')
           AT TIME ZONE 'Europe/Minsk'
FROM _seed_clock c
         CROSS JOIN (VALUES
                         ('demo.alena',
                          '$argon2id$v=19$m=65536,t=2,p=1$ymEyaoCVZ3vdCW8PQS5MaQ$8Y0wExGHrIaboJ0NiP1qLv9mhvyHiTr8Qbv1LdC/f2I',
                          'user', 'Алёна Сергеева', 'demo.alena@example.com'),
                         ('demo.maksim',
                          '$argon2id$v=19$m=65536,t=2,p=1$OfPZ/PSWqJaRkDKJKIAtoA$yCnShAPcpvyL8/asr4CJxbBXutfzNg6tZG6MAq3cjmg',
                          'user', 'Максим Орлов', 'demo.maksim@example.com'),
                         ('demo.admin',
                          '$argon2id$v=19$m=65536,t=2,p=1$o2XTdtLiMOPW+Txoyd9CTA$tIYAdVKHzkPwADlX/9poqK9m8Nv4tDUTSzjzKREOBfU',
                          'admin', 'Администратор 2D2', 'demo.admin@example.com')
) AS v(login, password_hash, role, full_name, email)
ON CONFLICT ((lower(login))) DO UPDATE
    SET password_hash = EXCLUDED.password_hash,
        updated_at = now()
WHERE users.email = EXCLUDED.email
  AND users.role = EXCLUDED.role;

-- Две абонентские записи; администратор сознательно НЕ имеет номера.
INSERT INTO subscribers
(user_id, account_number, msisdn, nickname, tariff_id,
 balance_byn, is_blocked, admin_blocked, created_at, updated_at)
SELECT u.id, v.account_number, v.msisdn, v.nickname, t.id,
       0.00, FALSE, v.admin_blocked,
       (c.month_local - INTERVAL '2 months' + INTERVAL '9 hours')
           AT TIME ZONE 'Europe/Minsk',
       (c.month_local - INTERVAL '2 months' + INTERVAL '9 hours')
           AT TIME ZONE 'Europe/Minsk'
FROM _seed_clock c
         CROSS JOIN (VALUES
                         ('demo.alena', '2D2-DEMO-0001', '+375291000101', 'Алёна', 'comfort-s', FALSE),
                         ('demo.maksim', '2D2-DEMO-0002', '+375291000102', 'Максим', 'comfort-m', TRUE)
) AS v(login, account_number, msisdn, nickname, tariff_code, admin_blocked)
         JOIN users u ON lower(u.login) = v.login
         JOIN tariffs t ON t.code = v.tariff_code
ON CONFLICT DO NOTHING;

-- Защита от случайного связывания демо-логина с не той ролью или номером.
DO $validate_accounts$
    BEGIN
        IF (SELECT count(*) FROM users
            WHERE lower(login) IN ('demo.alena','demo.maksim') AND role = 'user') <> 2
            OR (SELECT count(*) FROM users
                WHERE lower(login) = 'demo.admin' AND role = 'admin') <> 1
            OR (SELECT count(*)
                FROM subscribers s JOIN users u ON u.id = s.user_id
                WHERE (lower(u.login) = 'demo.alena' AND s.account_number = '2D2-DEMO-0001')
                   OR (lower(u.login) = 'demo.maksim' AND s.account_number = '2D2-DEMO-0002')) <> 2
            OR (SELECT count(*) FROM users
                WHERE (lower(login), lower(email)) IN (
                                                       ('demo.alena', 'demo.alena@example.com'),
                                                       ('demo.maksim', 'demo.maksim@example.com'),
                                                       ('demo.admin', 'demo.admin@example.com')
                    )) <> 3
            OR EXISTS (SELECT 1 FROM subscribers s JOIN users u ON u.id = s.user_id
                       WHERE lower(u.login) = 'demo.admin')
        THEN
            RAISE EXCEPTION 'Обнаружены конфликтующие демо-учётные записи. Транзакция отменена.';
        END IF;
    END
$validate_accounts$;

-- Опорный месяц первой загрузки: пользователь создан ровно за 2 месяца
-- до начального периода. На следующих запусках он не меняется.
CREATE TEMP TABLE _seed_anchor ON COMMIT DROP AS
SELECT date_trunc('month', created_at AT TIME ZONE 'Europe/Minsk')
           + INTERVAL '2 months' AS month_local
FROM users
WHERE lower(login) = 'demo.alena';

-- --------------------------------------------------------------------------
-- 3. Подключения услуг с историей. Начальный месяц фиксируется один раз
--    по созданию demo.alena, чтобы повторный запуск в следующем месяце
--    не создавал повторных подключений.
--    В демо-модели платные услуги начисляются за месяц пересечения,
--    без пропорционального перерасчёта за неполный месяц.
-- --------------------------------------------------------------------------
WITH wanted AS (
    SELECT s.id AS subscriber_id, svc.id AS service_id,
           (c.month_local + make_interval(months => v.start_m,
                                          days => v.start_d - 1, hours => 10))
               AT TIME ZONE 'Europe/Minsk' AS connected_at,
           CASE WHEN v.end_m IS NULL THEN NULL::timestamptz
                ELSE (c.month_local + make_interval(months => v.end_m,
                                                    days => v.end_d - 1, hours => 18))
                    AT TIME ZONE 'Europe/Minsk' END AS disconnected_at
    FROM _seed_anchor c
             CROSS JOIN (VALUES
                             ('demo.alena',  'messengers-unlimited',  -2, 2, NULL::int, NULL::int),
                             ('demo.alena',  'calls-other-60',        -2, 2, NULL, NULL),
                             ('demo.alena',  'safe-internet',         -2, 4, NULL, NULL),
                             ('demo.alena',  'roaming-calls',         -2, 6, -2, 16),
                             ('demo.alena',  'extra-internet-1gb',   -1, 10, -1, 26),
                             ('demo.maksim', 'internet-unlimited-5', -2, 2, NULL, NULL),
                             ('demo.maksim', 'calls-own-unlimited',  -2, 3, NULL, NULL),
                             ('demo.maksim', 'safe-internet',         -2, 4, -1, 4),
                             ('demo.maksim', 'roaming-internet-1gb', -2, 8, -2, 16),
                             ('demo.maksim', 'extra-internet-5gb',   -1, 3, -1, 16),
                             ('demo.maksim', 'anti-spam',             0, 3, NULL, NULL)
    ) AS v(login, service_code, start_m, start_d, end_m, end_d)
             JOIN users u ON lower(u.login) = v.login
             JOIN subscribers s ON s.user_id = u.id
             JOIN services svc ON svc.code = v.service_code
    WHERE (c.month_local + make_interval(months => v.start_m,
                                         days => v.start_d - 1, hours => 10))
              AT TIME ZONE 'Europe/Minsk' <= now()
)
INSERT INTO subscriber_services
(subscriber_id, service_id, connected_at, disconnected_at)
SELECT w.subscriber_id, w.service_id, w.connected_at, w.disconnected_at
FROM wanted w
WHERE NOT EXISTS (
    SELECT 1 FROM subscriber_services old
    WHERE old.subscriber_id = w.subscriber_id
      AND old.service_id = w.service_id
      AND old.connected_at = w.connected_at
);

-- --------------------------------------------------------------------------
-- 4. Детальная статистика пользования (день/время/тип ресурса).
--    От первого дня позапрошлого месяца до текущего времени, без будущих
--    записей. Значения детерминированные, но разнообразные по дням.
-- --------------------------------------------------------------------------
CREATE TEMP TABLE _seed_usage_raw ON COMMIT DROP AS
WITH days AS (
    SELECT gs.day_ts::date AS usage_day
    FROM _seed_clock c
             CROSS JOIN LATERAL generate_series(
            (c.month_local - INTERVAL '2 months')::date::timestamp,
            c.today_local::timestamp,
            INTERVAL '1 day') AS gs(day_ts)
)
SELECT v.login, v.metric, v.service_code,
       v.quantity::numeric(12,3) AS quantity,
       v.cost_byn::numeric(12,2) AS cost_byn,
       v.peer_number,
       (d.usage_day::timestamp
           + make_interval(hours => v.hour_of_day, mins => v.minute_of_hour))
           AT TIME ZONE 'Europe/Minsk' AS occurred_at
FROM days d
         CROSS JOIN LATERAL (
    VALUES
        ('demo.alena', 'internet_mb', NULL::text,
         8, 10, 190 + mod(EXTRACT(DAY FROM d.usage_day)::int, 6) * 14,
         0.00::numeric, NULL::text, NULL::int),
        ('demo.alena', 'internet_mb', NULL,
         19, 15, 235 + mod(EXTRACT(DAY FROM d.usage_day)::int, 5) * 17,
         0.00, NULL, NULL),
        ('demo.alena', 'messenger_mb', 'messengers-unlimited',
         13, 15, 62 + mod(EXTRACT(DAY FROM d.usage_day)::int, 6) * 3,
         0.00, NULL, NULL),
        ('demo.alena', 'minutes_other', 'calls-other-60',
         12, 30, 4 + mod(EXTRACT(DAY FROM d.usage_day)::int, 5),
         0.00, '+375291100401', 3),
        ('demo.alena', 'minutes_own', NULL,
         18, 20, 9 + mod(EXTRACT(DAY FROM d.usage_day)::int, 3),
         0.00, '+375291100402', 4),
        ('demo.maksim', 'internet_mb', 'internet-unlimited-5',
         9, 10, 360 + mod(EXTRACT(DAY FROM d.usage_day)::int, 7) * 22,
         0.00, NULL, NULL),
        ('demo.maksim', 'internet_mb', 'internet-unlimited-5',
         20, 10, 385 + mod(EXTRACT(DAY FROM d.usage_day)::int, 5) * 25,
         0.00, NULL, NULL),
        ('demo.maksim', 'minutes_own', 'calls-own-unlimited',
         17, 10, 11 + mod(EXTRACT(DAY FROM d.usage_day)::int, 4),
         0.00, '+375291100403', 4),
        ('demo.maksim', 'minutes_other', NULL,
         15, 40, 6 + mod(EXTRACT(DAY FROM d.usage_day)::int, 5),
         1.20, '+375291100404', 7)
    ) AS v(login, metric, service_code, hour_of_day, minute_of_hour,
           quantity, cost_byn, peer_number, every_n_days)
WHERE (v.every_n_days IS NULL
    OR mod(EXTRACT(DAY FROM d.usage_day)::int, v.every_n_days) = 0)
  AND (d.usage_day::timestamp
    + make_interval(hours => v.hour_of_day, mins => v.minute_of_hour))
          AT TIME ZONE 'Europe/Minsk' <= now() - INTERVAL '10 minutes';

-- Реальное использование коротких роуминговых пакетов в начале истории.
INSERT INTO _seed_usage_raw
(login, metric, service_code, quantity, cost_byn, peer_number, occurred_at)
SELECT 'demo.alena', 'minutes_roaming', 'roaming-calls',
       3.500::numeric(12,3), 0.00::numeric(12,2), '+375291100405',
       (c.month_local - INTERVAL '2 months'
           + make_interval(days => v.day_number - 1, hours => 14, mins => 30))
           AT TIME ZONE 'Europe/Minsk'
FROM _seed_anchor c
         CROSS JOIN (VALUES (7), (9), (12)) v(day_number)
WHERE (c.month_local - INTERVAL '2 months'
    + make_interval(days => v.day_number - 1, hours => 14, mins => 30))
          AT TIME ZONE 'Europe/Minsk' <= now() - INTERVAL '10 minutes';

INSERT INTO _seed_usage_raw
(login, metric, service_code, quantity, cost_byn, peer_number, occurred_at)
SELECT 'demo.maksim', 'internet_mb', 'roaming-internet-1gb',
       160.000::numeric(12,3), 0.00::numeric(12,2), NULL,
       (c.month_local - INTERVAL '2 months'
           + make_interval(days => v.day_number - 1, hours => 14, mins => 45))
           AT TIME ZONE 'Europe/Minsk'
FROM _seed_anchor c
         CROSS JOIN (VALUES (9), (12)) v(day_number)
WHERE (c.month_local - INTERVAL '2 months'
    + make_interval(days => v.day_number - 1, hours => 14, mins => 45))
          AT TIME ZONE 'Europe/Minsk' <= now() - INTERVAL '10 minutes';

INSERT INTO usage_events
(subscriber_id, service_id, metric, quantity, cost_byn,
 peer_number, occurred_at, created_at)
SELECT s.id, svc.id, r.metric, r.quantity, r.cost_byn,
       r.peer_number, r.occurred_at, r.occurred_at + INTERVAL '1 minute'
FROM _seed_usage_raw r
         JOIN users u ON lower(u.login) = r.login
         JOIN subscribers s ON s.user_id = u.id
         LEFT JOIN services svc ON svc.code = r.service_code
WHERE NOT EXISTS (
    SELECT 1 FROM usage_events old
    WHERE old.subscriber_id = s.id
      AND old.metric = r.metric
      AND old.occurred_at = r.occurred_at
      AND old.peer_number IS NOT DISTINCT FROM r.peer_number
);

-- --------------------------------------------------------------------------
-- 5. Финансовые начисления: тариф/платные услуги/платные события.
--    В завершённых месяцах фактические начисления позднее связываются
--    со счетами. За текущий месяц invoice_id остаётся NULL.
-- --------------------------------------------------------------------------
CREATE TEMP TABLE _seed_periods ON COMMIT DROP AS
SELECT (c.month_local + make_interval(months => gs.mo))::date AS period_start,
       gs.mo AS month_offset
FROM _seed_clock c
         CROSS JOIN generate_series(-2, 0) AS gs(mo);

-- Тариф: полная месячная стоимость включительно с НДС.
WITH wanted AS (
    SELECT s.id AS subscriber_id, p.period_start, t.monthly_fee_byn,
           'Тариф: ' || t.name AS description,
           (p.period_start::timestamp + INTERVAL '1 hour')
               AT TIME ZONE 'Europe/Minsk' AS charged_at
    FROM _seed_periods p
             JOIN subscribers s ON TRUE
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
             JOIN tariffs t ON t.id = s.tariff_id
    WHERE (p.period_start::timestamp + INTERVAL '1 hour')
              AT TIME ZONE 'Europe/Minsk' <= now()
)
INSERT INTO charges
(subscriber_id, period_start, charge_type, description,
 amount_byn, vat_byn, charged_at)
SELECT w.subscriber_id, w.period_start, 'tariff', w.description,
       round(w.monthly_fee_byn / 1.20, 2),
       w.monthly_fee_byn - round(w.monthly_fee_byn / 1.20, 2),
       w.charged_at
FROM wanted w
WHERE NOT EXISTS (
    SELECT 1 FROM charges old
    WHERE old.subscriber_id = w.subscriber_id
      AND old.period_start = w.period_start
      AND old.charge_type = 'tariff'
      AND old.description = w.description
);

-- Платные услуги: один платёж за месяц, в котором услуга была подключена.
WITH wanted AS (
    SELECT s.id AS subscriber_id, p.period_start,
           svc.monthly_fee_byn,
           'Услуга: ' || svc.name AS description,
           GREATEST(ss.connected_at,
                    (p.period_start::timestamp + INTERVAL '2 hours')
                        AT TIME ZONE 'Europe/Minsk') AS charged_at
    FROM _seed_periods p
             JOIN subscribers s ON TRUE
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
             JOIN subscriber_services ss ON ss.subscriber_id = s.id
             JOIN services svc ON svc.id = ss.service_id AND svc.monthly_fee_byn > 0
    WHERE ss.connected_at < ((p.period_start + INTERVAL '1 month')::timestamp
        AT TIME ZONE 'Europe/Minsk')
      AND (ss.disconnected_at IS NULL OR ss.disconnected_at >
                                         (p.period_start::timestamp AT TIME ZONE 'Europe/Minsk'))
      AND GREATEST(ss.connected_at,
                   (p.period_start::timestamp + INTERVAL '2 hours')
                       AT TIME ZONE 'Europe/Minsk') <= now()
)
INSERT INTO charges
(subscriber_id, period_start, charge_type, description,
 amount_byn, vat_byn, charged_at)
SELECT w.subscriber_id, w.period_start, 'service', w.description,
       round(w.monthly_fee_byn / 1.20, 2),
       w.monthly_fee_byn - round(w.monthly_fee_byn / 1.20, 2),
       w.charged_at
FROM wanted w
WHERE NOT EXISTS (
    SELECT 1 FROM charges old
    WHERE old.subscriber_id = w.subscriber_id
      AND old.period_start = w.period_start
      AND old.charge_type = 'service'
      AND old.description = w.description
);

-- Платные вызовы: отдельная строка начисления на каждый факт потребления.
WITH wanted AS (
    SELECT e.subscriber_id,
           date_trunc('month', e.occurred_at AT TIME ZONE 'Europe/Minsk')::date
                                                AS period_start,
           'Сверхпакетный вызов ' ||
           to_char(e.occurred_at AT TIME ZONE 'Europe/Minsk',
                   'YYYY-MM-DD HH24:MI') AS description,
           e.cost_byn,
           e.occurred_at + INTERVAL '2 minutes' AS charged_at
    FROM usage_events e
             JOIN subscribers s ON s.id = e.subscriber_id
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
             JOIN _seed_usage_raw r ON r.occurred_at = e.occurred_at
        AND r.metric = e.metric AND lower(u.login) = r.login
    WHERE e.cost_byn > 0
      AND e.occurred_at + INTERVAL '2 minutes' <= now()
)
INSERT INTO charges
(subscriber_id, period_start, charge_type, description,
 amount_byn, vat_byn, charged_at)
SELECT w.subscriber_id, w.period_start, 'usage', w.description,
       round(w.cost_byn / 1.20, 2),
       w.cost_byn - round(w.cost_byn / 1.20, 2),
       w.charged_at
FROM wanted w
WHERE NOT EXISTS (
    SELECT 1 FROM charges old
    WHERE old.subscriber_id = w.subscriber_id
      AND old.period_start = w.period_start
      AND old.charge_type = 'usage'
      AND old.description = w.description
);

-- --------------------------------------------------------------------------
-- 6. Демонстрационные платежи и разные статусы обработки.
-- --------------------------------------------------------------------------
WITH wanted AS (
    SELECT s.id AS subscriber_id, v.slot,
           v.amount_byn::numeric(12,2) AS amount_byn, v.status,
           (c.month_local + make_interval(months => v.month_offset,
                                          days => v.day_of_month - 1, hours => 11))
               AT TIME ZONE 'Europe/Minsk' AS created_at
    FROM _seed_anchor c
             CROSS JOIN (VALUES
                             ('demo.alena',  'alena-p1',  -2, 5, 35.00, 'succeeded'),
                             ('demo.alena',  'alena-p2',  -1, 9, 24.00, 'succeeded'),
                             ('demo.alena',  'alena-p3',   0, 6, 30.00, 'succeeded'),
                             ('demo.alena',  'alena-p4',   0, 18, 12.00, 'pending'),
                             ('demo.maksim', 'maksim-p1', -2, 14, 30.00, 'succeeded'),
                             ('demo.maksim', 'maksim-p2', -1, 19, 35.00, 'succeeded'),
                             ('demo.maksim', 'maksim-p3',  0, 9, 45.00, 'succeeded'),
                             ('demo.maksim', 'maksim-p4',  0, 14, 15.00, 'failed'),
                             ('demo.maksim', 'maksim-p5',  0, 20, 40.00, 'pending')
    ) AS v(login, slot, month_offset, day_of_month, amount_byn, status)
             JOIN users u ON lower(u.login) = v.login
             JOIN subscribers s ON s.user_id = u.id
)
INSERT INTO payments
(subscriber_id, idempotency_key, provider_payment_id, amount_byn,
 status, created_at, confirmed_at)
SELECT w.subscriber_id,
       'demo-2d2-' || to_char(w.created_at AT TIME ZONE 'Europe/Minsk', 'YYYYMM')
           || '-' || w.slot,
       CASE WHEN w.status = 'pending' THEN NULL
            ELSE 'simulated-2d2-' || to_char(w.created_at AT TIME ZONE 'Europe/Minsk', 'YYYYMM')
                     || '-' || w.slot END,
       w.amount_byn, w.status, w.created_at,
       CASE WHEN w.status = 'succeeded'
                THEN w.created_at + INTERVAL '3 minutes'
            ELSE NULL END
FROM wanted w
WHERE w.created_at + INTERVAL '3 minutes' <= now()
ON CONFLICT (subscriber_id, idempotency_key) DO NOTHING;

-- --------------------------------------------------------------------------
-- 7. Счета только за завершённые месяцы, суммы строго из charges.
-- --------------------------------------------------------------------------
INSERT INTO invoices
(subscriber_id, period_start, total_amount_byn, paid_amount_byn, issued_at)
SELECT s.id, p.period_start, 0.00, 0.00,
       ((p.period_start + INTERVAL '1 month 1 day')::timestamp
           + INTERVAL '10 hours') AT TIME ZONE 'Europe/Minsk'
FROM _seed_periods p
         JOIN subscribers s ON TRUE
         JOIN users u ON u.id = s.user_id
    AND lower(u.login) IN ('demo.alena', 'demo.maksim')
WHERE p.month_offset < 0
ON CONFLICT (subscriber_id, period_start) DO NOTHING;

-- Связываем начисления соответствующего месяца с его счётом.
UPDATE charges c
SET invoice_id = i.id
FROM invoices i
         JOIN subscribers s ON s.id = i.subscriber_id
         JOIN users u ON u.id = s.user_id
    AND lower(u.login) IN ('demo.alena', 'demo.maksim')
WHERE c.subscriber_id = i.subscriber_id
  AND c.period_start = i.period_start
  AND c.invoice_id IS NULL;

-- Пересчитываем сумму выставленного счёта из связанных строк начислений.
WITH totals AS (
    SELECT i.id AS invoice_id,
           COALESCE(sum(c.amount_byn + c.vat_byn), 0)::numeric(12,2) AS gross
    FROM invoices i
             JOIN subscribers s ON s.id = i.subscriber_id
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
             LEFT JOIN charges c ON c.invoice_id = i.id
    GROUP BY i.id
)
UPDATE invoices i
SET total_amount_byn = t.gross,
    paid_amount_byn = LEAST(i.paid_amount_byn, t.gross)
FROM totals t
WHERE t.invoice_id = i.id;

-- Распределяем подтверждённые пополнения по старым счетам (FIFO).
-- Демонстрационная упрощённая модель учёта, не эмуляция банковского webhook.
WITH confirmed AS (
    SELECT p.subscriber_id, sum(p.amount_byn) AS paid_total
    FROM payments p
             JOIN subscribers s ON s.id = p.subscriber_id
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
    WHERE p.status = 'succeeded'
    GROUP BY p.subscriber_id
), ordered AS (
    SELECT i.id,
           i.total_amount_byn,
           COALESCE(sum(i.total_amount_byn) OVER (
               PARTITION BY i.subscriber_id
               ORDER BY i.period_start
               ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING
               ), 0)::numeric(12,2) AS older_due,
           COALESCE(p.paid_total, 0)::numeric(12,2) AS paid_total
    FROM invoices i
             JOIN subscribers s ON s.id = i.subscriber_id
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
             LEFT JOIN confirmed p ON p.subscriber_id = i.subscriber_id
)
UPDATE invoices i
SET paid_amount_byn = GREATEST(
        0::numeric,
        LEAST(o.total_amount_byn, o.paid_total - o.older_due)
                      )
FROM ordered o
WHERE i.id = o.id;

-- Расчёт баланса в рамках трёхмесячной демонстрационной модели:
-- сумма подтверждённых пополнений минус все начисления с НДС,
-- включая текущие начисления до выставления счёта.
WITH totals AS (
    SELECT s.id,
           COALESCE((SELECT sum(p.amount_byn)
                     FROM payments p
                     WHERE p.subscriber_id = s.id
                       AND p.status = 'succeeded'), 0)
               - COALESCE((SELECT sum(c.amount_byn + c.vat_byn)
                           FROM charges c
                           WHERE c.subscriber_id = s.id), 0) AS calculated_balance
    FROM subscribers s
             JOIN users u ON u.id = s.user_id
        AND lower(u.login) IN ('demo.alena', 'demo.maksim')
)
UPDATE subscribers s
SET balance_byn = t.calculated_balance::numeric(12,2),
    updated_at = now()
FROM totals t
WHERE s.id = t.id
  AND s.balance_byn IS DISTINCT FROM t.calculated_balance;

-- --------------------------------------------------------------------------
-- 8. Избранные контакты и уведомления.
-- --------------------------------------------------------------------------
INSERT INTO favorite_contacts (subscriber_id, name, phone, created_at)
SELECT s.id, v.contact_name, v.contact_phone,
       s.created_at + INTERVAL '2 days'
FROM (VALUES
          ('demo.alena',  'Мама', '+375291100401'),
          ('demo.alena',  'Работа', '+375291100402'),
          ('demo.maksim', 'Семья', '+375291100403'),
          ('demo.maksim', 'Коллега', '+375291100404')
     ) AS v(login, contact_name, contact_phone)
         JOIN users u ON lower(u.login) = v.login
         JOIN subscribers s ON s.user_id = u.id
WHERE NOT EXISTS (
    SELECT 1 FROM favorite_contacts old
    WHERE old.subscriber_id = s.id AND old.phone = v.contact_phone
);

-- Уведомления: приветствие, счета, платежи и актуальные предупреждения.
CREATE TEMP TABLE _seed_notices ON COMMIT DROP AS
SELECT s.id AS subscriber_id, 'message'::varchar(10) AS kind,
       'Добро пожаловать в 2D2'::varchar(150) AS title,
       'Ваш номер подключён. Здесь можно управлять тарифом, услугами и расходами.'::text AS body,
       '/profile'::varchar(255) AS target_path,
       s.created_at + INTERVAL '1 hour' AS created_at,
       s.created_at + INTERVAL '1 day' AS read_at
FROM subscribers s
         JOIN users u ON u.id = s.user_id
    AND lower(u.login) IN ('demo.alena', 'demo.maksim')
UNION ALL
SELECT i.subscriber_id, 'message', 'Сформирован счёт',
       'Доступен счёт за ' || to_char(i.period_start, 'MM.YYYY') ||
       ' на сумму ' || to_char(i.total_amount_byn, 'FM999999990.00') || ' BYN.',
       '/finances', i.issued_at + INTERVAL '10 minutes',
       CASE WHEN i.period_start <
                 (SELECT month_local::date - INTERVAL '1 month' FROM _seed_clock)
                THEN i.issued_at + INTERVAL '1 day' ELSE NULL END
FROM invoices i
         JOIN subscribers s ON s.id = i.subscriber_id
         JOIN users u ON u.id = s.user_id
    AND lower(u.login) IN ('demo.alena', 'demo.maksim')
UNION ALL
SELECT p.subscriber_id, 'message',
       CASE WHEN p.status = 'succeeded' THEN 'Баланс пополнен'
            WHEN p.status = 'failed' THEN 'Ошибка пополнения'
            ELSE 'Ожидается платёж' END,
       'Операция на ' || to_char(p.amount_byn, 'FM999999990.00') ||
       ' BYN: ' || CASE WHEN p.status = 'succeeded' THEN 'подтверждена.'
                        WHEN p.status = 'failed' THEN 'не выполнена.'
                        ELSE 'ожидает подтверждения.' END,
       '/finances',
       COALESCE(p.confirmed_at, p.created_at) + INTERVAL '5 minutes',
       CASE WHEN p.created_at < (SELECT month_local - INTERVAL '1 month' FROM _seed_clock)
                THEN p.created_at + INTERVAL '1 day' ELSE NULL END
FROM payments p
         JOIN subscribers s ON s.id = p.subscriber_id
         JOIN users u ON u.id = s.user_id
    AND lower(u.login) IN ('demo.alena', 'demo.maksim');

-- Оповещение о достижении 80% интернет-пакета в текущем месяце.
INSERT INTO _seed_notices
(subscriber_id, kind, title, body, target_path, created_at, read_at)
SELECT s.id, 'alert', 'Заканчивается интернет-пакет',
       'В текущем месяце использовано ' ||
       to_char(round(sum(e.quantity), 0), 'FM999999990') ||
       ' МБ из ' || t.included_internet_mb || ' МБ.',
       '/analytics',
       (c.month_local + INTERVAL '19 days 14 hours') AT TIME ZONE 'Europe/Minsk',
       NULL
FROM _seed_clock c
         JOIN subscribers s ON TRUE
         JOIN users u ON u.id = s.user_id AND lower(u.login) = 'demo.alena'
         JOIN tariffs t ON t.id = s.tariff_id
         JOIN usage_events e ON e.subscriber_id = s.id AND e.metric = 'internet_mb'
WHERE t.included_internet_mb > 0
  AND e.occurred_at >= (c.month_local AT TIME ZONE 'Europe/Minsk')
  AND e.occurred_at < ((c.month_local + INTERVAL '1 month') AT TIME ZONE 'Europe/Minsk')
  AND (c.month_local + INTERVAL '19 days 14 hours')
          AT TIME ZONE 'Europe/Minsk' <= now()
GROUP BY s.id, t.included_internet_mb, c.month_local
HAVING sum(e.quantity) >= t.included_internet_mb * 0.8;

-- Второй абонент: задолженность и административная блокировка.
INSERT INTO _seed_notices
(subscriber_id, kind, title, body, target_path, created_at, read_at)
SELECT s.id, 'alert', 'Отрицательный баланс',
       'Баланс ниже нуля. Пополните лицевой счёт для погашения задолженности.',
       '/finances',
       (c.month_local + INTERVAL '16 days 9 hours') AT TIME ZONE 'Europe/Minsk',
       NULL
FROM _seed_clock c
         JOIN subscribers s ON s.balance_byn < 0
         JOIN users u ON u.id = s.user_id AND lower(u.login) = 'demo.maksim'
WHERE (c.month_local + INTERVAL '16 days 9 hours')
          AT TIME ZONE 'Europe/Minsk' <= now();

INSERT INTO _seed_notices
(subscriber_id, kind, title, body, target_path, created_at, read_at)
SELECT s.id, 'alert', 'Номер ограничен оператором',
       'Установлена административная блокировка номера. Обратитесь в поддержку.',
       '/profile',
       (c.month_local + INTERVAL '16 days 10 hours') AT TIME ZONE 'Europe/Minsk',
       NULL
FROM _seed_clock c
         JOIN subscribers s ON s.admin_blocked
         JOIN users u ON u.id = s.user_id AND lower(u.login) = 'demo.maksim'
WHERE (c.month_local + INTERVAL '16 days 10 hours')
          AT TIME ZONE 'Europe/Minsk' <= now();

INSERT INTO notifications
(subscriber_id, kind, title, body, target_path, created_at, read_at)
SELECT n.subscriber_id, n.kind, n.title, n.body,
       n.target_path, n.created_at, n.read_at
FROM _seed_notices n
WHERE n.created_at <= now()
  AND NOT EXISTS (
    SELECT 1 FROM notifications old
    WHERE old.subscriber_id = n.subscriber_id
      AND old.title = n.title
      AND old.created_at = n.created_at
);

-- --------------------------------------------------------------------------
-- 9. Контроль целостности перед фиксацией.
-- --------------------------------------------------------------------------
DO $validation$
    BEGIN
        IF EXISTS (
            SELECT 1
            FROM invoices i
                     JOIN subscribers s ON s.id = i.subscriber_id
                     JOIN users u ON u.id = s.user_id
                AND lower(u.login) IN ('demo.alena','demo.maksim')
                     LEFT JOIN charges c ON c.invoice_id = i.id
            GROUP BY i.id, i.total_amount_byn
            HAVING i.total_amount_byn <> COALESCE(sum(c.amount_byn + c.vat_byn), 0)
        ) THEN
            RAISE EXCEPTION 'Seed: total_amount_byn не совпадает с charges.';
        END IF;
        IF EXISTS (
            SELECT 1
            FROM charges c
                     JOIN subscribers s ON s.id = c.subscriber_id
                     JOIN users u ON u.id = s.user_id
                AND lower(u.login) IN ('demo.alena','demo.maksim')
                     JOIN _seed_clock clock ON TRUE
            WHERE c.period_start = clock.month_local::date
              AND c.invoice_id IS NOT NULL
        ) THEN
            RAISE EXCEPTION 'Seed: текущий месяц не должен иметь invoice_id.';
        END IF;
    END
$validation$;

COMMIT;

-- Краткий итог; роли/пароли самого приложения проверяются в backend.
SELECT u.login, u.role, u.full_name, s.msisdn, s.balance_byn,
       s.is_blocked, s.admin_blocked
FROM telecom.users u
         LEFT JOIN telecom.subscribers s ON s.user_id = u.id
WHERE lower(u.login) IN ('demo.alena','demo.maksim','demo.admin')
ORDER BY u.role, u.login;
