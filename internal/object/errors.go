package object

import "errors"

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return e.Code + ": " + e.Message
}

func NewError(status int, code, message string) error {
	return &APIError{Status: status, Code: code, Message: message}
}

func IsError(err error, code string) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Code == code
}
