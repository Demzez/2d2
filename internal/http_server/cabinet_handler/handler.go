package cabinet_handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"telecom/internal/http_server/common"
	"telecom/internal/service/cabinet"
)

const op = "telecom.internal.http_server.cabinet_handler"

type Handler struct {
	log     *slog.Logger
	service *cabinet.Service
}

func NewHandler(log *slog.Logger, service *cabinet.Service) *Handler {
	return &Handler{log: log, service: service}
}

func (h *Handler) GetMe() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetMe()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetMe(r.Context(), common.Subscriber(r))
		return value, 200, err
	})
}

func (h *Handler) ChangeMe() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeMe()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			FullName *string `json:"full_name"`
			Email    *string `json:"email"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.ChangeMe(r.Context(), common.Subscriber(r), input.FullName, input.Email)
		return value, 200, err
	})
}

func (h *Handler) ChangeNumber() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeNumber()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Nickname  *string `json:"nickname"`
			IsBlocked *bool   `json:"is_blocked"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.ChangeNumber(r.Context(), common.Subscriber(r), input.Nickname, input.IsBlocked)
		return value, 200, err
	})
}

func (h *Handler) GetDashboard() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetDashboard()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetDashboard(r.Context(), common.Subscriber(r))
		return value, 200, err
	})
}

func (h *Handler) GetTariffs() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetTariffs()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetTariffs(r.Context(), common.Subscriber(r))
		return value, 200, err
	})
}

func (h *Handler) ChangeTariff() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeTariff()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			TariffID int64 `json:"tariff_id"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.ChangeTariff(r.Context(), common.Subscriber(r), input.TariffID)
		return value, 200, err
	})
}

func (h *Handler) GetServices() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetServices()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		value, err := h.service.GetServices(r.Context(), common.Subscriber(r), q.Get("status"), q.Get("search"), q.Get("category"))
		return value, 200, err
	})
}

func (h *Handler) ChangeService() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeService()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		if input.Enabled == nil {
			return nil, 0, common.Fail("Укажите enabled")
		}
		value, err := h.service.ChangeService(r.Context(), common.Subscriber(r), id, *input.Enabled)
		return value, 200, err
	})
}

func (h *Handler) GetFavorites() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetFavorites()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetFavorites(r.Context(), common.Subscriber(r))
		return value, 200, err
	})
}

func (h *Handler) SaveFavorite() http.HandlerFunc {
	return common.Wrap(h.log, op+".SaveFavorite()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Name  string `json:"name"`
			Phone string `json:"phone"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.SaveFavorite(r.Context(), common.Subscriber(r), input.Name, input.Phone)
		return value, 201, err
	})
}

func (h *Handler) ChangeFavorite() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeFavorite()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		var input struct {
			Name  *string `json:"name"`
			Phone *string `json:"phone"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.ChangeFavorite(r.Context(), common.Subscriber(r), id, input.Name, input.Phone)
		return value, 200, err
	})
}

func (h *Handler) RemoveFavorite() http.HandlerFunc {
	return common.Wrap(h.log, op+".RemoveFavorite()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		if err := h.service.RemoveFavorite(r.Context(), common.Subscriber(r), id); err != nil {
			return nil, 0, err
		}
		return nil, 204, nil
	})
}

func (h *Handler) GetNotifications() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetNotifications()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		unreadOnly := false
		if q.Has("unread_only") {
			var err error
			unreadOnly, err = strconv.ParseBool(q.Get("unread_only"))
			if err != nil {
				return nil, 0, common.Fail("Некорректный unread_only")
			}
		}
		value, err := h.service.GetNotifications(r.Context(), common.Subscriber(r), q.Get("kind"), unreadOnly)
		return value, 200, err
	})
}

func (h *Handler) ChangeNotification() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangeNotification()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.ChangeNotification(r.Context(), common.Subscriber(r), id)
		return value, 200, err
	})
}

func (h *Handler) GetFinanceSummary() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetFinanceSummary()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetFinanceSummary(r.Context(), common.Subscriber(r))
		return value, 200, err
	})
}

func (h *Handler) GetCharges() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetCharges()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		month, err := common.Month(r.URL.Query().Get("month"), cabinet.MonthNow())
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetCharges(r.Context(), common.Subscriber(r), month)
		return value, 200, err
	})
}

func (h *Handler) GetInvoices() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetInvoices()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		months, err := common.IntRange(r.URL.Query().Get("months"), 12, 1, 12)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetInvoices(r.Context(), common.Subscriber(r), months)
		return value, 200, err
	})
}

func (h *Handler) GetInvoice() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetInvoice()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetInvoice(r.Context(), common.Subscriber(r), id)
		return value, 200, err
	})
}

func (h *Handler) GetPayments() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetPayments()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		months, err := common.IntRange(r.URL.Query().Get("months"), 8, 1, 12)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetPayments(r.Context(), common.Subscriber(r), months)
		return value, 200, err
	})
}

func (h *Handler) GetUsageDetails() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetUsageDetails()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		loc, _ := time.LoadLocation("Europe/Minsk")
		to := time.Now().In(loc).Format("2006-01-02")
		from := time.Now().In(loc).AddDate(0, -1, 0).Format("2006-01-02")
		if q.Has("from") {
			from = q.Get("from")
		}
		if q.Has("to") {
			to = q.Get("to")
		}
		first, err := time.Parse("2006-01-02", from)
		if err != nil || first.Format("2006-01-02") != from {
			return nil, 0, common.Fail("Некорректная дата from")
		}
		last, err := time.Parse("2006-01-02", to)
		if err != nil || last.Format("2006-01-02") != to {
			return nil, 0, common.Fail("Некорректная дата to")
		}
		if last.Before(first) || last.After(first.AddDate(3, 0, 0)) {
			return nil, 0, common.Fail("Диапазон должен быть не более трёх лет")
		}
		page, err := common.IntRange(q.Get("page"), 1, 1, 1000000)
		if err != nil {
			return nil, 0, err
		}
		limit, err := common.IntRange(q.Get("limit"), 50, 1, 100)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetUsageDetails(r.Context(), common.Subscriber(r), from, to, page, limit)
		return value, 200, err
	})
}

func (h *Handler) GetAnalytics() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetAnalytics()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		month, err := common.Month(q.Get("month"), cabinet.MonthNow())
		if err != nil {
			return nil, 0, err
		}
		months, err := common.IntRange(q.Get("months"), 6, 1, 12)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetAnalytics(r.Context(), common.Subscriber(r), month, months)
		return value, 200, err
	})
}
