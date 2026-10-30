package cabinet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"telecom/internal/object"
	"telecom/internal/service/auth"
)

const op = "2d2.internal.service.cabinet"

type Repository interface {
	ReadMe(context.Context, int64) (json.RawMessage, error)
	UpdateMe(context.Context, int64, *string, *string) (json.RawMessage, error)
	UpdateNumber(context.Context, int64, *string, *bool) (json.RawMessage, error)
	ReadTariffs(context.Context, int64) (json.RawMessage, error)
	UpdateTariff(context.Context, int64, int64) (json.RawMessage, error)
	ReadServices(context.Context, int64, string, string, string) (json.RawMessage, error)
	UpdateService(context.Context, int64, int64, bool) (json.RawMessage, error)
	ReadDashboard(context.Context, int64, string) (json.RawMessage, error)
	ReadFavorites(context.Context, int64) (json.RawMessage, error)
	CreateFavorite(context.Context, int64, string, string) (json.RawMessage, error)
	UpdateFavorite(context.Context, int64, int64, *string, *string) (json.RawMessage, error)
	DeleteFavorite(context.Context, int64, int64) error
	ReadNotifications(context.Context, int64, string, bool) (json.RawMessage, error)
	UpdateNotification(context.Context, int64, int64) (json.RawMessage, error)
	ReadFinanceSummary(context.Context, int64, string) (json.RawMessage, error)
	ReadCharges(context.Context, int64, string) (json.RawMessage, error)
	ReadInvoices(context.Context, int64, int, string) (json.RawMessage, error)
	ReadInvoice(context.Context, int64, int64) (json.RawMessage, error)
	ReadPayments(context.Context, int64, int, string) (json.RawMessage, error)
	ReadUsageDetails(context.Context, int64, string, string, int, int) (json.RawMessage, error)
	ReadAnalytics(context.Context, int64, string, int) (json.RawMessage, error)
}

type Service struct{ rep Repository }

func NewService(rep Repository) *Service { return &Service{rep: rep} }

func MonthNow() string {
	loc, _ := time.LoadLocation("Europe/Minsk")
	return time.Now().In(loc).Format("2006-01")
}

func (s *Service) GetMe(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadMe(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s.GetMe() | reading profile failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeMe(ctx context.Context, id int64, name, email *string) (json.RawMessage, error) {
	if name == nil && email == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите поле для обновления")
	}
	if name != nil && (len(strings.TrimSpace(*name)) < 2 || len(*name) > 150) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректное имя")
	}
	if email != nil && (*email != "" && (!strings.Contains(*email, "@") || len(*email) > 254)) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный email")
	}
	value, err := s.rep.UpdateMe(ctx, id, name, email)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeMe() | updating profile failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeNumber(ctx context.Context, id int64, nickname *string, blocked *bool) (json.RawMessage, error) {
	if nickname == nil && blocked == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите поле для обновления")
	}
	if nickname != nil && len(*nickname) > 100 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Слишком длинное название")
	}
	value, err := s.rep.UpdateNumber(ctx, id, nickname, blocked)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeNumber() | updating number failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetTariffs(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadTariffs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s.GetTariffs() | reading tariffs failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeTariff(ctx context.Context, id, tariffID int64) (json.RawMessage, error) {
	if tariffID < 1 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный тариф")
	}
	value, err := s.rep.UpdateTariff(ctx, id, tariffID)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeTariff() | changing tariff failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetServices(ctx context.Context, id int64, status, search, category string) (json.RawMessage, error) {
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "connected" && status != "available" {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный status")
	}
	if category != "" && !strings.Contains("|internet|messengers|calls|security|roaming|other|", "|"+category+"|") {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректная категория")
	}
	if len(search) > 150 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Слишком длинная строка поиска")
	}
	value, err := s.rep.ReadServices(ctx, id, status, search, category)
	if err != nil {
		return nil, fmt.Errorf("%s.GetServices() | reading services failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeService(ctx context.Context, id, serviceID int64, enabled bool) (json.RawMessage, error) {
	if serviceID < 1 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректная услуга")
	}
	value, err := s.rep.UpdateService(ctx, id, serviceID, enabled)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeService() | changing connection failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetDashboard(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadDashboard(ctx, id, MonthNow())
	if err != nil {
		return nil, fmt.Errorf("%s.GetDashboard() | dashboard failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetFavorites(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadFavorites(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s.GetFavorites() | reading contacts failed: %w", op, err)
	}
	return value, nil
}

func validContact(name string, phone string) bool {
	return len(strings.TrimSpace(name)) > 0 && len(name) <= 100 && auth.ValidPhone(phone)
}

func (s *Service) SaveFavorite(ctx context.Context, id int64, name, phone string) (json.RawMessage, error) {
	if !validContact(name, phone) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Неверное имя или номер")
	}
	value, err := s.rep.CreateFavorite(ctx, id, name, phone)
	if err != nil {
		return nil, fmt.Errorf("%s.SaveFavorite() | creating contact failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeFavorite(ctx context.Context, subID, id int64, name, phone *string) (json.RawMessage, error) {
	if name == nil && phone == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите поле для обновления")
	}
	if name != nil && (len(strings.TrimSpace(*name)) == 0 || len(*name) > 100) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Неверное имя")
	}
	if phone != nil && !auth.ValidPhone(*phone) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Неверный телефон")
	}
	value, err := s.rep.UpdateFavorite(ctx, subID, id, name, phone)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeFavorite() | updating contact failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) RemoveFavorite(ctx context.Context, subID, id int64) error {
	if err := s.rep.DeleteFavorite(ctx, subID, id); err != nil {
		return fmt.Errorf("%s.RemoveFavorite() | deleting contact failed: %w", op, err)
	}
	return nil
}

func (s *Service) GetNotifications(ctx context.Context, id int64, kind string, unreadOnly bool) (json.RawMessage, error) {
	if kind == "" {
		kind = "all"
	}
	if kind != "all" && kind != "alert" && kind != "message" {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Неверный kind")
	}
	value, err := s.rep.ReadNotifications(ctx, id, kind, unreadOnly)
	if err != nil {
		return nil, fmt.Errorf("%s.GetNotifications() | reading notifications failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) ChangeNotification(ctx context.Context, id, notificationID int64) (json.RawMessage, error) {
	value, err := s.rep.UpdateNotification(ctx, id, notificationID)
	if err != nil {
		return nil, fmt.Errorf("%s.ChangeNotification() | marking notification failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetFinanceSummary(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadFinanceSummary(ctx, id, MonthNow())
	if err != nil {
		return nil, fmt.Errorf("%s.GetFinanceSummary() | reading summary failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetCharges(ctx context.Context, id int64, month string) (json.RawMessage, error) {
	value, err := s.rep.ReadCharges(ctx, id, month)
	if err != nil {
		return nil, fmt.Errorf("%s.GetCharges() | reading charges failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetInvoices(ctx context.Context, id int64, months int) (json.RawMessage, error) {
	value, err := s.rep.ReadInvoices(ctx, id, months, MonthNow())
	if err != nil {
		return nil, fmt.Errorf("%s.GetInvoices() | reading invoices failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetInvoice(ctx context.Context, id, invoiceID int64) (json.RawMessage, error) {
	value, err := s.rep.ReadInvoice(ctx, id, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("%s.GetInvoice() | reading invoice failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetPayments(ctx context.Context, id int64, months int) (json.RawMessage, error) {
	value, err := s.rep.ReadPayments(ctx, id, months, MonthNow())
	if err != nil {
		return nil, fmt.Errorf("%s.GetPayments() | reading payments failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetUsageDetails(ctx context.Context, id int64, from, to string, page, limit int) (json.RawMessage, error) {
	value, err := s.rep.ReadUsageDetails(ctx, id, from, to, page, limit)
	if err != nil {
		return nil, fmt.Errorf("%s.GetUsageDetails() | reading usage failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetAnalytics(ctx context.Context, id int64, month string, months int) (json.RawMessage, error) {
	value, err := s.rep.ReadAnalytics(ctx, id, month, months)
	if err != nil {
		return nil, fmt.Errorf("%s.GetAnalytics() | reading analytics failed: %w", op, err)
	}
	return value, nil
}
