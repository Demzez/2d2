package main

import (
	"log/slog"
	"net/http"
	"strings"

	"telecom/internal/config"
	"telecom/internal/http_server/auth_handler"
	"telecom/internal/http_server/cabinet_handler"
	"telecom/internal/http_server/common"
	"telecom/internal/http_server/finance_handler"
	"telecom/internal/object"
	"telecom/internal/service/auth"
	"telecom/internal/service/cabinet"
	"telecom/internal/service/finance"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Services struct {
	auth    *auth.Service
	cabinet *cabinet.Service
	finance *finance.Service
}

func createRouter(logger *slog.Logger, cfg *config.Config, services Services) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if cfg.AllowedOrigin != "" && req.Header.Get("Origin") == cfg.AllowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", cfg.AllowedOrigin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Payment-Signature")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			}
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	a := auth_handler.NewHandler(logger, services.auth)
	c := cabinet_handler.NewHandler(logger, services.cabinet)
	f := finance_handler.NewHandler(logger, services.finance)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/otp/request", a.RequestOTP())
		r.Post("/auth/register", a.Register())
		r.Post("/auth/login", a.Login())
		r.Post("/auth/otp/verify", a.VerifyOTP())
		r.Post("/auth/refresh", a.Refresh())
		r.Post("/auth/logout", a.Logout())
		r.Post("/integrations/payment-webhook", f.ChangePayment())

		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					const op = "telecom.cmd.telecom.createRouter.authorize"
					bearer := req.Header.Get("Authorization")
					if !strings.HasPrefix(bearer, "Bearer ") {
						common.HandleError(logger.With(slog.String("op", op)), w, object.NewError(401, "UNAUTHORIZED", "Требуется Bearer-токен"))
						return
					}
					id, err := services.auth.Authorize(req.Context(), strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer ")))
					if err != nil {
						common.HandleError(logger.With(slog.String("op", op)), w, err)
						return
					}
					next.ServeHTTP(w, req.WithContext(common.WithSubscriber(req.Context(), id)))
				})
			})
			r.Get("/me", c.GetMe())
			r.Patch("/me", c.ChangeMe())
			r.Patch("/number", c.ChangeNumber())
			r.Get("/dashboard", c.GetDashboard())
			r.Get("/tariffs", c.GetTariffs())
			r.Put("/number/tariff", c.ChangeTariff())
			r.Get("/services", c.GetServices())
			r.Put("/services/{id}/connection", c.ChangeService())
			r.Get("/finance/summary", c.GetFinanceSummary())
			r.Get("/finance/charges", c.GetCharges())
			r.Get("/finance/invoices", c.GetInvoices())
			r.Get("/finance/invoices/{id}", c.GetInvoice())
			r.Get("/finance/payments", c.GetPayments())
			r.Get("/finance/usage-details", c.GetUsageDetails())
			r.Post("/finance/topups", f.SaveTopup())
			r.Get("/analytics", c.GetAnalytics())
			r.Get("/favorites", c.GetFavorites())
			r.Post("/favorites", c.SaveFavorite())
			r.Patch("/favorites/{id}", c.ChangeFavorite())
			r.Delete("/favorites/{id}", c.RemoveFavorite())
			r.Get("/notifications", c.GetNotifications())
			r.Patch("/notifications/{id}/read", c.ChangeNotification())
		})
	})
	return r
}
