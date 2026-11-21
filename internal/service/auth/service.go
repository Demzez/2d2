package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"telecom/internal/http_server/common"
	"time"
	"unicode/utf8"

	"telecom/internal/config"
	"telecom/internal/object"
	"telecom/internal/repository/postgres"

	"golang.org/x/crypto/argon2"
)

const op = "2d2.internal.service.auth"

var phoneRE = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
var loginRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,50}$`)

type Repository interface {
	ReadLogin(context.Context, string) (postgres.LoginRow, error)
	ReadPhoneUser(context.Context, string) (int64, string, error)
	ReadPrincipal(context.Context, int64) (int64, string, error)
	CreateOTP(context.Context, string, string, string) error
	VerifyOTP(context.Context, string, string, string) (int64, error)
	CreateAccount(context.Context, string, string, string, string, string, string) (json.RawMessage, int64, error)
	CreateSession(context.Context, int64, string) error
	UpdateSession(context.Context, string, string) (int64, string, error)
	DeleteSession(context.Context, string) error
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Role         string `json:"role"`
}

type Service struct {
	rep Repository
	cfg *config.Config
}

func NewService(rep Repository, cfg *config.Config) *Service {
	return &Service{rep: rep, cfg: cfg}
}

func ValidPhone(phone string) bool { return phoneRE.MatchString(phone) }

func (s *Service) otpHash(phone, purpose, code string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.OTPSecret))
	mac.Write([]byte(phone + ":" + purpose + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomBytes(size int) ([]byte, error) {
	b := make([]byte, size)
	_, err := rand.Read(b)
	return b, err
}

func hashPassword(password string) (string, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, 2, 64*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func checkPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=2,p=1" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 2, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (s *Service) RequestOTP(ctx context.Context, phone, purpose string) error {
	const op = op + ".RequestOTP()"
	if !ValidPhone(phone) || (purpose != "login" && purpose != "register") {
		return object.NewError(400, "VALIDATION_ERROR", "Некорректный номер или назначение кода")
	}
	if !s.cfg.DevMode {
		return object.NewError(503, "SMS_UNAVAILABLE", "SMS доступен только в локальном учебном режиме")
	}

	_, existingRole, err := s.rep.ReadPhoneUser(ctx, phone)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s | phone lookup failed: %w", op, err)
	}
	canDeliver := (purpose == "login" && exists && existingRole == "user") || (purpose == "register" && !exists)
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Errorf("%s | OTP generation failed: %w", op, err)
	}
	code := fmt.Sprintf("%06d", n.Int64())
	if err := s.rep.CreateOTP(ctx, phone, purpose, s.otpHash(phone, purpose, code)); err != nil {
		return fmt.Errorf("%s | OTP creation failed: %w", op, err)
	}
	// Deliberately not routed through slog or HTTP: explicit development-only mock SMS console.
	if canDeliver {
		fmt.Printf("[DEV MOCK SMS] %s (%s): %s\n", phone, purpose, code)
	}
	return nil
}

func (s *Service) Login(ctx context.Context, login, password string) (Tokens, error) {
	const op = op + ".Login()"
	if login == "" || password == "" {
		return Tokens{}, object.NewError(401, "INVALID_CREDENTIALS", "Неверный логин или пароль")
	}
	row, err := s.rep.ReadLogin(ctx, login)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tokens{}, object.NewError(401, "INVALID_CREDENTIALS", "Неверный логин или пароль")
		}
		return Tokens{}, fmt.Errorf("%s | lookup failed: %w", op, err)
	}
	if !checkPassword(password, row.PasswordHash) {
		return Tokens{}, object.NewError(401, "INVALID_CREDENTIALS", "Неверный логин или пароль")
	}
	return s.newTokens(ctx, row.UserID, row.Role)
}

func (s *Service) VerifyOTP(ctx context.Context, phone, code string) (Tokens, error) {
	const op = op + ".VerifyOTP()"
	if !ValidPhone(phone) || len(code) != 6 || !onlyDigits(code) {
		return Tokens{}, object.NewError(401, "INVALID_OTP", "Неверный код")
	}
	userID, err := s.rep.VerifyOTP(ctx, phone, "login", s.otpHash(phone, "login", code))
	if err != nil {
		return Tokens{}, fmt.Errorf("%s | OTP failed: %w", op, err)
	}
	return s.newTokens(ctx, userID, "user")
}

type Registration struct {
	Login           string `json:"login"`
	Password        string `json:"password"`
	FullName        string `json:"full_name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	OTPCode         string `json:"otp_code"`
	PrivacyAccepted bool   `json:"privacy_accepted"`
}

func (s *Service) Register(ctx context.Context, data Registration) (map[string]any, error) {
	const op = op + ".Register()"
	if !loginRE.MatchString(data.Login) || !ValidPhone(data.Phone) || len(strings.TrimSpace(data.FullName)) < 2 || utf8.RuneCountInString(strings.TrimSpace(data.FullName)) > 150 || len(data.Password) < 8 || len(data.Password) > 128 || (len(data.OTPCode) != 6 || !onlyDigits(data.OTPCode)) || !data.PrivacyAccepted || (data.Email != "" && (!strings.Contains(data.Email, "@") || len(data.Email) > 254)) {
		return nil, object.NewError(400, "VALIDATION_ERROR", "Некорректные данные регистрации")
	}
	passwordHash, err := hashPassword(data.Password)
	if err != nil {
		return nil, fmt.Errorf("%s | password hash failed: %w", op, err)
	}
	account, userID, err := s.rep.CreateAccount(ctx, data.Login, strings.TrimSpace(data.FullName), data.Email, data.Phone, passwordHash, s.otpHash(data.Phone, "register", data.OTPCode))
	if err != nil {
		return nil, fmt.Errorf("%s | account creation failed: %w", op, err)
	}
	tokens, err := s.newTokens(ctx, userID, "user")
	if err != nil {
		return nil, err
	}
	var response map[string]any
	if err := json.Unmarshal(account, &response); err != nil {
		return nil, fmt.Errorf("%s | account decode failed: %w", op, err)
	}
	response["role"] = "user"
	response["access_token"] = tokens.AccessToken
	response["refresh_token"] = tokens.RefreshToken
	response["expires_in"] = tokens.ExpiresIn
	return response, nil
}

func (s *Service) newTokens(ctx context.Context, userID int64, role string) (Tokens, error) {
	const op = op + ".newTokens()"
	refreshBytes, err := randomBytes(32)
	if err != nil {
		return Tokens{}, fmt.Errorf("%s | token generation failed: %w", op, err)
	}
	refresh := base64.RawURLEncoding.EncodeToString(refreshBytes)
	if err := s.rep.CreateSession(ctx, userID, digest(refresh)); err != nil {
		return Tokens{}, fmt.Errorf("%s | creating session failed: %w", op, err)
	}
	return Tokens{
		AccessToken:  s.signJWT(userID, role),
		RefreshToken: refresh,
		ExpiresIn:    900,
		Role:         role,
	}, nil
}

func (s *Service) signJWT(userID int64, role string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"sub":  strconv.FormatInt(userID, 10),
		"role": role,
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
	})
	payload := base64.RawURLEncoding.EncodeToString(claims)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) Authorize(ctx context.Context, token string) (common.Principal, error) {
	const op = op + ".Authorize()"
	parts := strings.Split(token, ".")
	unauthorized := object.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "Требуется авторизация")
	if len(parts) != 3 {
		return common.Principal{}, unauthorized
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(actual, mac.Sum(nil)) {
		return common.Principal{}, unauthorized
	}
	if parts[0] != base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) {
		return common.Principal{}, unauthorized
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return common.Principal{}, unauthorized
	}
	var claims struct {
		Sub     string `json:"sub"`
		Expires int64  `json:"exp"`
		Issued  int64  `json:"iat"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Expires <= time.Now().Unix() || claims.Issued > time.Now().Unix()+30 {
		return common.Principal{}, unauthorized
	}
	userID, err := strconv.ParseInt(claims.Sub, 10, 64)
	if err != nil || userID < 1 {
		return common.Principal{}, unauthorized
	}
	subID, role, err := s.rep.ReadPrincipal(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return common.Principal{}, unauthorized
	}
	if err != nil {
		return common.Principal{}, fmt.Errorf("%s | reading principal failed: %w", op, err)
	}
	return common.Principal{UserID: userID, SubscriberID: subID, Role: role}, nil
}

func (s *Service) Refresh(ctx context.Context, oldToken string) (Tokens, error) {
	const op = op + ".Refresh()"
	if oldToken == "" {
		return Tokens{}, object.NewError(401, "UNAUTHORIZED", "Сессия недействительна")
	}
	b, err := randomBytes(32)
	if err != nil {
		return Tokens{}, fmt.Errorf("%s | generating token failed: %w", op, err)
	}
	fresh := base64.RawURLEncoding.EncodeToString(b)
	userID, role, err := s.rep.UpdateSession(ctx, digest(oldToken), digest(fresh))
	if err != nil {
		return Tokens{}, fmt.Errorf("%s | refreshing session failed: %w", op, err)
	}
	return Tokens{
		AccessToken:  s.signJWT(userID, role),
		RefreshToken: fresh,
		ExpiresIn:    900,
		Role:         role,
	}, nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return object.NewError(400, "VALIDATION_ERROR", "Не указан refresh_token")
	}
	return s.rep.DeleteSession(ctx, digest(token))
}

func onlyDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
