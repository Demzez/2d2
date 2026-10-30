package finance_handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"telecom/internal/http_server/common"
	"telecom/internal/object"
	"telecom/internal/repository/postgres"
	"telecom/internal/service/finance"
)

const op = "2d2.internal.http_server.finance_handler"

type Handler struct {
	log     *slog.Logger
	service *finance.Service
}

func NewHandler(log *slog.Logger, service *finance.Service) *Handler {
	return &Handler{log: log, service: service}
}

func (h *Handler) SaveTopup() http.HandlerFunc {
	return common.Wrap(h.log, op+".SaveTopup()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Amount string `json:"amount_byn"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.SaveTopup(r.Context(), common.Subscriber(r), r.Header.Get("Idempotency-Key"), input.Amount)
		return value, http.StatusCreated, err
	})
}

func (h *Handler) ChangePayment() http.HandlerFunc {
	return common.Wrap(h.log, op+".ChangePayment()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, 0, object.NewError(400, "VALIDATION_ERROR", "Некорректное событие платежа")
		}
		var event postgres.PaymentEvent
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			return nil, 0, object.NewError(400, "VALIDATION_ERROR", "Некорректное событие платежа")
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return nil, 0, object.NewError(400, "VALIDATION_ERROR", "Ожидался один JSON-объект")
		}
		if err := h.service.ChangePayment(r.Context(), r.Header.Get("X-Payment-Signature"), body, event); err != nil {
			return nil, 0, err
		}
		return nil, http.StatusNoContent, nil
	})
}
