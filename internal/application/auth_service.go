package application

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"note-app/internal/domain"
	appauth "note-app/internal/infrastructure/auth"

	"github.com/google/uuid"
)

type UserRepository interface {
	Create(ctx context.Context, user domain.User) (domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, userID string) (domain.User, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error)
	GetByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	RevokeByID(ctx context.Context, tokenID string, revokedAt time.Time) error
}

type RegisterInput struct {
	Email    string
	Password string
	FullName string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthTokens struct {
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

type AuthResult struct {
	User   domain.User `json:"user"`
	Tokens AuthTokens  `json:"tokens"`
}

type AuthService struct {
	users           UserRepository
	refreshTokens   RefreshTokenRepository
	passwordManager appauth.PasswordManager
	tokenManager    appauth.TokenManager
	refreshTokenTTL time.Duration
}

func NewAuthService(users UserRepository, refreshTokens RefreshTokenRepository, passwordManager appauth.PasswordManager, tokenManager appauth.TokenManager, refreshTokenTTL time.Duration) AuthService {
	return AuthService{
		users:           users,
		refreshTokens:   refreshTokens,
		passwordManager: passwordManager,
		tokenManager:    tokenManager,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (s AuthService) Register(ctx context.Context, input RegisterInput) (domain.User, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return domain.User{}, fmt.Errorf("%w: invalid email", domain.ErrValidation)
	}
	fullName := strings.TrimSpace(input.FullName)
	if fullName == "" {
		return domain.User{}, fmt.Errorf("%w: full_name is required", domain.ErrValidation)
	}
	if err := validatePassword(input.Password); err != nil {
		return domain.User{}, err
	}

	_, err = s.users.GetByEmail(ctx, email)
	switch {
	case err == nil:
		return domain.User{}, domain.ErrEmailAlreadyUsed
	case !errors.Is(err, domain.ErrNotFound):
		return domain.User{}, err
	}

	passwordHash, err := s.passwordManager.Hash(input.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:           uuid.NewString(),
		Email:        email,
		FullName:     fullName,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	created, err := s.users.Create(ctx, user)
	if err != nil {
		return domain.User{}, err
	}

	return sanitizeUser(created), nil
}

func (s AuthService) Login(ctx context.Context, input LoginInput) (AuthResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return AuthResult{}, domain.ErrInvalidCredentials
	}

	user, err := s.users.GetByEmail(ctx, email)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return AuthResult{}, domain.ErrInvalidCredentials
	case err != nil:
		return AuthResult{}, err
	}

	if err := s.passwordManager.Compare(user.PasswordHash, input.Password); err != nil {
		return AuthResult{}, domain.ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{User: sanitizeUser(user), Tokens: tokens}, nil
}

func (s AuthService) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	hash := s.tokenManager.HashRefreshToken(strings.TrimSpace(refreshToken))
	stored, err := s.refreshTokens.GetByHash(ctx, hash)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return AuthResult{}, domain.ErrUnauthorized
	case err != nil:
		return AuthResult{}, err
	}
	if stored.RevokedAt != nil {
		return AuthResult{}, domain.ErrUnauthorized
	}
	if time.Now().UTC().After(stored.ExpiresAt) {
		return AuthResult{}, domain.ErrTokenExpired
	}

	if err := s.refreshTokens.RevokeByID(ctx, stored.ID, time.Now().UTC()); err != nil {
		return AuthResult{}, err
	}

	user, err := s.users.GetByID(ctx, stored.UserID)
	if err != nil {
		return AuthResult{}, err
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{User: sanitizeUser(user), Tokens: tokens}, nil
}

func (s AuthService) Logout(ctx context.Context, refreshToken string) error {
	hash := s.tokenManager.HashRefreshToken(strings.TrimSpace(refreshToken))
	stored, err := s.refreshTokens.GetByHash(ctx, hash)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	return s.refreshTokens.RevokeByID(ctx, stored.ID, time.Now().UTC())
}

func (s AuthService) CurrentUser(ctx context.Context, userID string) (domain.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	return sanitizeUser(user), nil
}

func (s AuthService) issueTokens(ctx context.Context, user domain.User) (AuthTokens, error) {
	now := time.Now().UTC()
	accessToken, accessExpiresAt, err := s.tokenManager.GenerateAccessToken(user.ID, user.Email, now)
	if err != nil {
		return AuthTokens{}, err
	}

	rawRefreshToken, refreshHash, err := s.tokenManager.NewRefreshToken()
	if err != nil {
		return AuthTokens{}, err
	}

	refreshExpiresAt := now.Add(s.refreshTokenTTL)
	if _, err := s.refreshTokens.Create(ctx, domain.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: refreshExpiresAt,
		CreatedAt: now,
	}); err != nil {
		return AuthTokens{}, err
	}

	return AuthTokens{
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          rawRefreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func sanitizeUser(user domain.User) domain.User {
	user.PasswordHash = ""
	return user
}

func normalizeEmail(input string) (string, error) {
	email := strings.TrimSpace(strings.ToLower(input))
	if _, err := mail.ParseAddress(email); err != nil {
		return "", err
	}
	return email, nil
}

func validatePassword(password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", domain.ErrValidation)
	}

	hasUpper := false
	hasLower := false
	hasNumber := false
	for _, r := range password {
		if r >= '0' && r <= '9' {
			hasNumber = true
		}
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber {
		return fmt.Errorf("%w: password must contain uppercase, lowercase, and numbers", domain.ErrValidation)
	}

	return nil
}
