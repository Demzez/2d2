package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"telecom/internal/object"
	"telecom/internal/repository/postgres"
)

const op = "telecom.internal.service.admin"

var codeRE = regexp.MustCompile(`^[A-Za-z0-9_-]{2,60}$`)

type Repository interface {
	ReadAdminOverview(context.Context) (json.RawMessage, error)
	ReadAdminSubscribers(context.Context, string, string, int, int) (json.RawMessage, error)
	ReadAdminSubscriber(context.Context, int64) (json.RawMessage, error)
	UpdateAdminSubscriberBlock(context.Context, int64, bool) (json.RawMessage, error)
	ReadAdminTariffs(context.Context, string, string) (json.RawMessage, error)
	CreateAdminTariff(context.Context, postgres.AdminTariffInput) (json.RawMessage, error)
	UpdateAdminTariff(context.Context, int64, postgres.AdminTariffPatch) (json.RawMessage, error)
	DeleteAdminTariff(context.Context, int64) error
	ReadAdminServices(context.Context, string, string, string) (json.RawMessage, error)
	ReadAdminServiceByID(context.Context, int64) (postgres.AdminServiceRow, error)
	CreateAdminService(context.Context, postgres.AdminServiceRow) (json.RawMessage, error)
	UpdateAdminService(context.Context, postgres.AdminServiceRow) (json.RawMessage, error)
	DeleteAdminService(context.Context, int64) error
}

type Service struct {
	rep Repository
}

func NewService(rep Repository) *Service {
	return &Service{rep: rep}
}

type NullableString struct {
	Set   bool
	Valid bool
	Value string
}

func (n *NullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Valid = false
		n.Value = ""
		return nil
	}
	n.Valid = true
	return json.Unmarshal(data, &n.Value)
}

type NullableInt struct {
	Set   bool
	Valid bool
	Value int
}

func (n *NullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Valid = false
		n.Value = 0
		return nil
	}
	n.Valid = true
	return json.Unmarshal(data, &n.Value)
}

type CreateTariffRequest struct {
	Code               *string `json:"code"`
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	MonthlyFeeBYN      *string `json:"monthly_fee_byn"`
	IncludedInternetMB *int    `json:"included_internet_mb"`
	IsFeatured         *bool   `json:"is_featured"`
	IsActive           *bool   `json:"is_active"`
}

type UpdateTariffRequest struct {
	Code               *string `json:"code"`
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	MonthlyFeeBYN      *string `json:"monthly_fee_byn"`
	IncludedInternetMB *int    `json:"included_internet_mb"`
	IsFeatured         *bool   `json:"is_featured"`
	IsActive           *bool   `json:"is_active"`
}

type CreateServiceRequest struct {
	Code          *string `json:"code"`
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	Category      *string `json:"category"`
	MonthlyFeeBYN *string `json:"monthly_fee_byn"`
	QuotaMetric   *string `json:"quota_metric"`
	QuotaAmount   *int    `json:"quota_amount"`
	IsUnlimited   *bool   `json:"is_unlimited"`
	IsFeatured    *bool   `json:"is_featured"`
	IsActive      *bool   `json:"is_active"`
}

type UpdateServiceRequest struct {
	Code          *string        `json:"code"`
	Name          *string        `json:"name"`
	Description   *string        `json:"description"`
	Category      *string        `json:"category"`
	MonthlyFeeBYN *string        `json:"monthly_fee_byn"`
	QuotaMetric   NullableString `json:"quota_metric"`
	QuotaAmount   NullableInt    `json:"quota_amount"`
	IsUnlimited   *bool          `json:"is_unlimited"`
	IsFeatured    *bool          `json:"is_featured"`
	IsActive      *bool          `json:"is_active"`
}

func (s *Service) GetOverview(ctx context.Context) (json.RawMessage, error) {
	value, err := s.rep.ReadAdminOverview(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s.GetOverview() | reading overview failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetSubscribers(ctx context.Context, search, status string, page, limit int) (json.RawMessage, error) {
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "blocked" {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный параметр status")
	}
	if len(search) > 150 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Слишком длинный поисковый запрос")
	}
	value, err := s.rep.ReadAdminSubscribers(ctx, strings.TrimSpace(search), status, page, limit)
	if err != nil {
		return nil, fmt.Errorf("%s.GetSubscribers() | reading subscribers failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetSubscriber(ctx context.Context, id int64) (json.RawMessage, error) {
	value, err := s.rep.ReadAdminSubscriber(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s.GetSubscriber() | reading subscriber failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) BlockSubscriber(ctx context.Context, id int64, adminBlocked bool) (json.RawMessage, error) {
	value, err := s.rep.UpdateAdminSubscriberBlock(ctx, id, adminBlocked)
	if err != nil {
		return nil, fmt.Errorf("%s.BlockSubscriber() | updating block failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) GetTariffs(ctx context.Context, search, status string) (json.RawMessage, error) {
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "inactive" {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный параметр status")
	}
	if len(search) > 150 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Слишком длинный поисковый запрос")
	}
	value, err := s.rep.ReadAdminTariffs(ctx, strings.TrimSpace(search), status)
	if err != nil {
		return nil, fmt.Errorf("%s.GetTariffs() | reading tariffs failed: %w", op, err)
	}
	return value, nil
}

func validTariffCode(code string) bool {
	return len(code) <= 50 && codeRE.MatchString(code)
}

func validMoneyNonNegative(amount string) bool {
	kopecks, err := postgres.MoneyToKopecks(amount)
	return err == nil && kopecks >= 0
}

func (s *Service) CreateTariff(ctx context.Context, req CreateTariffRequest) (json.RawMessage, error) {
	if req.Code == nil || req.Name == nil || req.MonthlyFeeBYN == nil || req.IncludedInternetMB == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите обязательные поля тарифа")
	}
	code := strings.TrimSpace(*req.Code)
	name := strings.TrimSpace(*req.Name)
	if !validTariffCode(code) || utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 120 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный код или название тарифа")
	}
	if !validMoneyNonNegative(*req.MonthlyFeeBYN) || *req.IncludedInternetMB < 0 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Стоимость и пакет интернета не могут быть отрицательными")
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	featured := false
	if req.IsFeatured != nil {
		featured = *req.IsFeatured
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	value, err := s.rep.CreateAdminTariff(ctx, postgres.AdminTariffInput{
		Code:               code,
		Name:               name,
		Description:        desc,
		MonthlyFeeBYN:      *req.MonthlyFeeBYN,
		IncludedInternetMB: *req.IncludedInternetMB,
		IsFeatured:         featured,
		IsActive:           active,
	})
	if err != nil {
		return nil, fmt.Errorf("%s.CreateTariff() | creating tariff failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) UpdateTariff(ctx context.Context, id int64, req UpdateTariffRequest) (json.RawMessage, error) {
	if req.Code == nil && req.Name == nil && req.Description == nil && req.MonthlyFeeBYN == nil &&
		req.IncludedInternetMB == nil && req.IsFeatured == nil && req.IsActive == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите хотя бы одно поле для обновления")
	}
	var patch postgres.AdminTariffPatch
	if req.Code != nil {
		c := strings.TrimSpace(*req.Code)
		if !validTariffCode(c) {
			return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный код тарифа")
		}
		patch.Code = &c
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if utf8.RuneCountInString(n) == 0 || utf8.RuneCountInString(n) > 120 {
			return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректное название тарифа")
		}
		patch.Name = &n
	}
	if req.Description != nil {
		patch.Description = req.Description
	}
	if req.MonthlyFeeBYN != nil {
		if !validMoneyNonNegative(*req.MonthlyFeeBYN) {
			return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректная стоимость тарифа")
		}
		patch.MonthlyFeeBYN = req.MonthlyFeeBYN
	}
	if req.IncludedInternetMB != nil {
		if *req.IncludedInternetMB < 0 {
			return nil, object.NewError(400, "VALIDATION_ERROR", "Пакет интернета не может быть отрицательным")
		}
		patch.IncludedInternetMB = req.IncludedInternetMB
	}
	patch.IsFeatured = req.IsFeatured
	patch.IsActive = req.IsActive

	value, err := s.rep.UpdateAdminTariff(ctx, id, patch)
	if err != nil {
		return nil, fmt.Errorf("%s.UpdateTariff() | updating tariff failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) DeleteTariff(ctx context.Context, id int64) error {
	if err := s.rep.DeleteAdminTariff(ctx, id); err != nil {
		return fmt.Errorf("%s.DeleteTariff() | deleting tariff failed: %w", op, err)
	}
	return nil
}

func validCategory(c string) bool {
	return strings.Contains("|internet|messengers|calls|security|roaming|other|", "|"+c+"|")
}

func validMetric(m string) bool {
	return strings.Contains("|internet_mb|messenger_mb|minutes_own|minutes_other|minutes_roaming|", "|"+m+"|")
}

func validateServiceConsistency(row postgres.AdminServiceRow) error {
	if !codeRE.MatchString(row.Code) || utf8.RuneCountInString(row.Name) == 0 || utf8.RuneCountInString(row.Name) > 150 {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректный код или название услуги")
	}
	if !validCategory(row.Category) {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректная категория услуги")
	}
	if !validMoneyNonNegative(row.MonthlyFeeBYN) {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректная стоимость услуги")
	}
	if row.QuotaMetric != nil && !validMetric(*row.QuotaMetric) {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректная метрика квоты")
	}
	if row.QuotaAmount != nil {
		if *row.QuotaAmount <= 0 || row.QuotaMetric == nil {
			return object.NewError(400, "VALIDATION_ERROR", "Некорректный лимит услуги")
		}
	}
	if row.IsUnlimited {
		if row.QuotaMetric == nil || row.QuotaAmount != nil {
			return object.NewError(400, "VALIDATION_ERROR", "Для безлимитной услуги требуется quota_metric и quota_amount = null")
		}
	}
	return nil
}

func (s *Service) GetServices(ctx context.Context, search, category, status string) (json.RawMessage, error) {
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "active" && status != "inactive" {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректный параметр status")
	}
	if category != "" && !validCategory(category) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректная категория")
	}
	if len(search) > 150 {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Слишком длинный поисковый запрос")
	}
	value, err := s.rep.ReadAdminServices(ctx, strings.TrimSpace(search), category, status)
	if err != nil {
		return nil, fmt.Errorf("%s.GetServices() | reading services failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) CreateService(ctx context.Context, req CreateServiceRequest) (json.RawMessage, error) {
	if req.Code == nil || req.Name == nil || req.Category == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите обязательные поля услуги")
	}
	row := postgres.AdminServiceRow{
		Code:          strings.TrimSpace(*req.Code),
		Name:          strings.TrimSpace(*req.Name),
		Description:   "",
		Category:      *req.Category,
		MonthlyFeeBYN: "0.00",
		QuotaMetric:   req.QuotaMetric,
		QuotaAmount:   req.QuotaAmount,
		IsUnlimited:   false,
		IsFeatured:    false,
		IsActive:      true,
	}
	if req.Description != nil {
		row.Description = *req.Description
	}
	if req.MonthlyFeeBYN != nil {
		row.MonthlyFeeBYN = *req.MonthlyFeeBYN
	}
	if req.IsUnlimited != nil {
		row.IsUnlimited = *req.IsUnlimited
	}
	if req.IsFeatured != nil {
		row.IsFeatured = *req.IsFeatured
	}
	if req.IsActive != nil {
		row.IsActive = *req.IsActive
	}
	if err := validateServiceConsistency(row); err != nil {
		return nil, err
	}
	value, err := s.rep.CreateAdminService(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("%s.CreateService() | creating service failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) UpdateService(ctx context.Context, id int64, req UpdateServiceRequest) (json.RawMessage, error) {
	if req.Code == nil && req.Name == nil && req.Description == nil && req.Category == nil &&
		req.MonthlyFeeBYN == nil && !req.QuotaMetric.Set && !req.QuotaAmount.Set &&
		req.IsUnlimited == nil && req.IsFeatured == nil && req.IsActive == nil {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Укажите хотя бы одно поле для обновления")
	}

	row, err := s.rep.ReadAdminServiceByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%s.UpdateService() | loading service failed: %w", op, err)
	}

	if req.Code != nil {
		row.Code = strings.TrimSpace(*req.Code)
	}
	if req.Name != nil {
		row.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		row.Description = *req.Description
	}
	if req.Category != nil {
		row.Category = *req.Category
	}
	if req.MonthlyFeeBYN != nil {
		row.MonthlyFeeBYN = *req.MonthlyFeeBYN
	}
	if req.QuotaMetric.Set {
		if req.QuotaMetric.Valid {
			v := strings.TrimSpace(req.QuotaMetric.Value)
			row.QuotaMetric = &v
		} else {
			row.QuotaMetric = nil
		}
	}
	if req.QuotaAmount.Set {
		if req.QuotaAmount.Valid {
			v := req.QuotaAmount.Value
			row.QuotaAmount = &v
		} else {
			row.QuotaAmount = nil
		}
	}
	if req.IsUnlimited != nil {
		row.IsUnlimited = *req.IsUnlimited
	}
	if req.IsFeatured != nil {
		row.IsFeatured = *req.IsFeatured
	}
	if req.IsActive != nil {
		row.IsActive = *req.IsActive
	}

	if err := validateServiceConsistency(row); err != nil {
		return nil, err
	}

	value, err := s.rep.UpdateAdminService(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("%s.UpdateService() | updating service failed: %w", op, err)
	}
	return value, nil
}

func (s *Service) DeleteService(ctx context.Context, id int64) error {
	if err := s.rep.DeleteAdminService(ctx, id); err != nil {
		return fmt.Errorf("%s.DeleteService() | deleting service failed: %w", op, err)
	}
	return nil
}
