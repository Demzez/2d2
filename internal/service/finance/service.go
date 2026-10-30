package finance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"telecom/internal/config"
	"telecom/internal/object"
	"telecom/internal/repository/postgres"
)

const op = "telecom.internal.service.finance"

var keyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)

type Repository interface {
	CreatePayment(context.Context, int64, string, string) (json.RawMessage, error)
	UpdatePayment(context.Context, postgres.PaymentEvent) error
}

type Service struct {
	rep Repository
	cfg *config.Config
}

func NewService(rep Repository, cfg *config.Config) *Service { return &Service{rep: rep, cfg: cfg} }

func (s *Service) SaveTopup(ctx context.Context, id int64, key, amount string) (json.RawMessage, error) {
	const op = op + ".SaveTopup()"
	if !s.cfg.DevMode {
		return nil, object.NewError(503, "PAYMENT_UNAVAILABLE", "Платёжный провайдер не подключён")
	}
	if !keyRE.MatchString(key) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный Idempotency-Key")
	}
	kopecks, err := postgres.MoneyToKopecks(amount)
	if err != nil {
		return nil, err
	}
	if kopecks < 100 || kopecks > 100000 {
		return nil, object.NewError(422, "VALIDATION_ERROR", "Сумма должна быть от 1.00 до 1000.00 BYN")
	}
	amount = fmt.Sprintf("%d.%02d", kopecks/100, kopecks%100)
	value, err := s.rep.CreatePayment(ctx, id, key, amount)
	if err != nil {
		return nil, fmt.Errorf("%s | initiating payment failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangePayment(ctx context.Context, signature string, body []byte, event postgres.PaymentEvent) error {
	const op = op + ".ChangePayment()"
	if !s.cfg.DevMode {
		return object.NewError(503, "PAYMENT_UNAVAILABLE", "Платёжный провайдер не подключён")
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.PaymentSecret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(want)) {
		return object.NewError(401, "UNAUTHORIZED", "Некорректная подпись платежа")
	}
	if event.EventID == "" || event.ProviderPaymentID == "" || event.Currency != "BYN" || (event.Status != "succeeded" && event.Status != "failed") {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректное событие платежа")
	}
	if _, err := postgres.MoneyToKopecks(event.Amount); err != nil {
		return err
	}
	if err := s.rep.UpdatePayment(ctx, event); err != nil {
		return fmt.Errorf("%s | payment confirmation failed: %w", op, err)
	}
	return nil
}
