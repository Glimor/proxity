package domain

import "fmt"

// AppError separates a stable machine-readable code from the user-facing text
// and internal detail.
type AppError struct {
	Code    string `json:"code"`
	UserMsg string `json:"user_msg"`
	Detail  string `json:"detail,omitempty"`
	cause   error
}

func (e *AppError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.UserMsg, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.UserMsg)
}

func (e *AppError) Unwrap() error { return e.cause }

func NewError(code, userMsg, detail string) *AppError {
	return &AppError{Code: code, UserMsg: userMsg, Detail: detail}
}

func WrapError(code, userMsg string, err error) *AppError {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return &AppError{Code: code, UserMsg: userMsg, Detail: detail, cause: err}
}
