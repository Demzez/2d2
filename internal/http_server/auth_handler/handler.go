package auth_handler

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"telecom/internal/http_server/common"
	"telecom/internal/object"
	"telecom/internal/service/auth"
)

const op = "2d2.internal.http_server.auth_handler"

type Handler struct {
	log      *slog.Logger
	service  *auth.Service
	mu       sync.Mutex
	requests map[string][]time.Time
}

func NewHandler(log *slog.Logger, service *auth.Service) *Handler {
	return &Handler{log: log, service: service, requests: make(map[string][]time.Time)}
}

func (h *Handler) rateLimited(addr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for key, dates := range h.requests {
		valid := dates[:0]
		for _, date := range dates {
			if now.Sub(date) < time.Hour {
				valid = append(valid, date)
			}
		}
		if len(valid) == 0 {
			delete(h.requests, key)
		} else {
			h.requests[key] = valid
		}
	}
	dates := h.requests[addr]
	if len(dates) >= 20 {
		return true
	}
	h.requests[addr] = append(dates, now)
	return false
}

func (h *Handler) RequestOTP() http.HandlerFunc {
	return common.Wrap(h.log, op+".RequestOTP()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Phone   string `json:"phone"`
			Purpose string `json:"purpose"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		// Do not trust proxy headers without a configured trusted reverse proxy.
		ip := r.RemoteAddr
		if pos := strings.LastIndex(ip, ":"); pos > 0 {
			ip = ip[:pos]
		}
		if h.rateLimited(ip) {
			return nil, 0, object.NewError(429, "RATE_LIMITED", "Слишком много запросов кода")
		}
		if err := h.service.RequestOTP(r.Context(), input.Phone, input.Purpose); err != nil {
			return nil, 0, err
		}
		return map[string]any{"status": "sent", "expires_in_seconds": 300}, 202, nil
	})
}

func (h *Handler) Register() http.HandlerFunc {
	return common.Wrap(h.log, op+".Register()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input auth.Registration
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.Register(r.Context(), input)
		return value, 201, err
	})
}

func (h *Handler) Login() http.HandlerFunc {
	return common.Wrap(h.log, op+".Login()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Login    string `json:"login"`
			Password string `json:"password"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.Login(r.Context(), input.Login, input.Password)
		return value, 200, err
	})
}

func (h *Handler) VerifyOTP() http.HandlerFunc {
	return common.Wrap(h.log, op+".VerifyOTP()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			Phone string `json:"phone"`
			Code  string `json:"code"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.VerifyOTP(r.Context(), input.Phone, input.Code)
		return value, 200, err
	})
}

func (h *Handler) Refresh() http.HandlerFunc {
	return common.Wrap(h.log, op+".Refresh()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		value, err := h.service.Refresh(r.Context(), input.RefreshToken)
		return value, 200, err
	})
}

func (h *Handler) Logout() http.HandlerFunc {
	return common.Wrap(h.log, op+".Logout()", func(_ http.ResponseWriter, r *http.Request) (any, int, error) {
		var input struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := common.Decode(r, &input); err != nil {
			return nil, 0, err
		}
		if err := h.service.Logout(r.Context(), input.RefreshToken); err != nil {
			return nil, 0, err
		}
		return nil, 204, nil
	})
}
