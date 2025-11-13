package service

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/bark-labs/bark-secure-proxy/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthService handles admin authentication and JWT issuance.
type AuthService struct {
	enabled  bool
	username string
	password string
	secret   []byte
}

// Claims represents JWT payload.
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// NewAuthService builds AuthService from config.
func NewAuthService(cfg *config.Config) *AuthService {
	authCfg := cfg.Auth
	username := strings.TrimSpace(authCfg.Username)
	if username == "" {
		username = "admin"
	}
	password := strings.TrimSpace(authCfg.Password)
	if password == "" {
		password = "admin123"
	}
	secret := strings.TrimSpace(authCfg.JWTSecret)
	if secret == "" {
		secret = "bark-secure-default-secret"
	}
	return &AuthService{
		enabled:  authCfg.Enabled,
		username: username,
		password: password,
		secret:   []byte(secret),
	}
}

// Enabled reports whether authentication is enforced.
func (a *AuthService) Enabled() bool {
	return a != nil && a.enabled
}

// Username returns configured admin username.
func (a *AuthService) Username() string {
	if a == nil {
		return ""
	}
	return a.username
}

// Authenticate validates user credentials and returns a JWT token.
func (a *AuthService) Authenticate(username, password string) (string, error) {
	if !a.Enabled() {
		return "", nil
	}
	if !a.matchUsername(username) || !a.matchPassword(password) {
		return "", errors.New("用户名或密码错误")
	}
	claims := Claims{
		Username: a.username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(12 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(a.secret)
	if err != nil {
		return "", err
	}
	return signed, nil
}

// Validate parses a token and returns its claims if valid.
func (a *AuthService) Validate(token string) (*Claims, error) {
	if !a.Enabled() {
		return &Claims{Username: "anonymous"}, nil
	}
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return a.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := parsed.Claims.(*Claims); ok && parsed.Valid {
		return claims, nil
	}
	return nil, errors.New("token 无效")
}

func (a *AuthService) matchUsername(input string) bool {
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(input)), []byte(a.username)) == 1
}

func (a *AuthService) matchPassword(input string) bool {
	if strings.HasPrefix(a.password, "$2a$") || strings.HasPrefix(a.password, "$2b$") || strings.HasPrefix(a.password, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(a.password), []byte(input)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(input), []byte(a.password)) == 1
}
