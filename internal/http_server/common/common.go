package common

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"telecom/internal/object"

	"github.com/go-chi/chi/v5"
)

type subscriberKey struct{}

func WithSubscriber(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, subscriberKey{}, id)
}

func Subscriber(r *http.Request) int64 {
	id, _ := r.Context().Value(subscriberKey{}).(int64)
	return id
}

func ID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		return 0, object.NewError(400, "VALIDATION_ERROR", "Некорректный идентификатор")
	}
	return id, nil
}

func Decode(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректное тело JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return object.NewError(400, "VALIDATION_ERROR", "Ожидался один JSON-объект")
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func HandleError(log *slog.Logger, w http.ResponseWriter, err error) {
	var apiError *object.APIError
	if errors.As(err, &apiError) {
		WriteJSON(w, apiError.Status, map[string]any{"error": map[string]string{"code": apiError.Code, "message": apiError.Message}})
		if apiError.Status >= 500 {
			log.Error("request failed", slog.String("error", err.Error()))
		}
		return
	}
	log.Error("request failed", slog.String("error", err.Error()))
	WriteJSON(w, 500, map[string]any{"error": map[string]string{"code": "INTERNAL_ERROR", "message": "Внутренняя ошибка сервера"}})
}

func Month(value, fallback string) (string, error) {
	if value == "" {
		return fallback, nil
	}
	if len(value) != 7 || value[4] != '-' {
		return "", object.NewError(400, "VALIDATION_ERROR", "Неверный формат месяца")
	}
	for _, c := range strings.ReplaceAll(value, "-", "") {
		if c < '0' || c > '9' {
			return "", object.NewError(400, "VALIDATION_ERROR", "Неверный формат месяца")
		}
	}
	if value[:4] == "0000" || value[5:] < "01" || value[5:] > "12" {
		return "", object.NewError(400, "VALIDATION_ERROR", "Неверный месяц")
	}
	return value, nil
}

func IntRange(value string, fallback, min, max int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	i, err := strconv.Atoi(value)
	if err != nil || i < min || i > max {
		return 0, object.NewError(400, "VALIDATION_ERROR", "Параметр вне допустимого диапазона")
	}
	return i, nil
}

func Fail(message string) error {
	return object.NewError(400, "VALIDATION_ERROR", message)
}

type Action func(http.ResponseWriter, *http.Request) (any, int, error)

func Wrap(logger *slog.Logger, operation string, action Action) http.HandlerFunc {
	log := logger.With(slog.String("op", operation))
	return func(w http.ResponseWriter, r *http.Request) {
		result, status, err := action(w, r)
		if err != nil {
			HandleError(log, w, err)
			return
		}
		if status == http.StatusNoContent {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		WriteJSON(w, status, result)
	}
}
