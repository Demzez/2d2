package admin_handler

import (
	"log/slog"
	"net/http"

	"telecom/internal/http_server/common"
	"telecom/internal/service/admin"
)

const op = "telecom.internal.http_server.admin_handler"

type Handler struct {
	log     *slog.Logger
	service *admin.Service
}

func NewHandler(log *slog.Logger, service *admin.Service) *Handler {
	return &Handler{log: log, service: service}
}

func (h *Handler) GetOverview() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetOverview()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		value, err := h.service.GetOverview(r.Context())
		return value, http.StatusOK, err
	})
}

func (h *Handler) GetSubscribers() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetSubscribers()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		page, err := common.IntRange(q.Get("page"), 1, 1, 1000000)
		if err != nil {
			return nil, 0, err
		}
		limit, err := common.IntRange(q.Get("limit"), 20, 1, 100)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetSubscribers(r.Context(), q.Get("search"), q.Get("status"), page, limit)
		return value, http.StatusOK, err
	})
}

func (h *Handler) GetSubscriber() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetSubscriber()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		value, err := h.service.GetSubscriber(r.Context(), id)
		return value, http.StatusOK, err
	})
}

func (h *Handler) BlockSubscriber() http.HandlerFunc {
	return common.Wrap(h.log, op+".BlockSubscriber()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		var input struct {
			AdminBlocked *bool `json:"admin_blocked"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		if input.AdminBlocked == nil {
			return nil, 0, common.Fail("Укажите admin_blocked")
		}
		value, err := h.service.BlockSubscriber(r.Context(), id, *input.AdminBlocked)
		return value, http.StatusOK, err
	})
}

func (h *Handler) GetTariffs() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetTariffs()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		value, err := h.service.GetTariffs(r.Context(), q.Get("search"), q.Get("status"))
		return value, http.StatusOK, err
	})
}

func (h *Handler) CreateTariff() http.HandlerFunc {
	return common.Wrap(h.log, op+".CreateTariff()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input admin.CreateTariffRequest
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.CreateTariff(r.Context(), input)
		return value, http.StatusCreated, err
	})
}

func (h *Handler) UpdateTariff() http.HandlerFunc {
	return common.Wrap(h.log, op+".UpdateTariff()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		var input admin.UpdateTariffRequest
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.UpdateTariff(r.Context(), id, input)
		return value, http.StatusOK, err
	})
}

func (h *Handler) DeleteTariff() http.HandlerFunc {
	return common.Wrap(h.log, op+".DeleteTariff()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		if err := h.service.DeleteTariff(r.Context(), id); err != nil {
			return nil, 0, err
		}
		return nil, http.StatusNoContent, nil
	})
}

func (h *Handler) GetServices() http.HandlerFunc {
	return common.Wrap(h.log, op+".GetServices()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		q := r.URL.Query()
		value, err := h.service.GetServices(r.Context(), q.Get("search"), q.Get("category"), q.Get("status"))
		return value, http.StatusOK, err
	})
}

func (h *Handler) CreateService() http.HandlerFunc {
	return common.Wrap(h.log, op+".CreateService()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input admin.CreateServiceRequest
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.CreateService(r.Context(), input)
		return value, http.StatusCreated, err
	})
}

func (h *Handler) UpdateService() http.HandlerFunc {
	return common.Wrap(h.log, op+".UpdateService()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		var input admin.UpdateServiceRequest
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.UpdateService(r.Context(), id, input)
		return value, http.StatusOK, err
	})
}

func (h *Handler) DeleteService() http.HandlerFunc {
	return common.Wrap(h.log, op+".DeleteService()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		id, err := common.ID(r)
		if err != nil {
			return nil, 0, err
		}
		if err := h.service.DeleteService(r.Context(), id); err != nil {
			return nil, 0, err
		}
		return nil, http.StatusNoContent, nil
	})
}
