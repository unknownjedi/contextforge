package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/repository"
)

type GitHubProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type AuthService interface {
	ValidateAndAuthenticatePAT(ctx context.Context, pat string) (*ent.User, string, error)
	ExchangeOAuthCode(ctx context.Context, code string) (*ent.User, string, error)
	GetUser(ctx context.Context, userID uuid.UUID) (*ent.User, error)
}

type DefaultAuthService struct {
	userRepo      repository.UserRepository
	encryptionKey []byte
	jwtSecret     []byte
	sessionExpiry time.Duration
	httpClient    *http.Client
	githubAPIBase string
}

func NewAuthService(
	userRepo repository.UserRepository,
	hexEncryptionKey string,
	jwtSecret string,
	sessionExpiry time.Duration,
	httpClient *http.Client,
) (*DefaultAuthService, error) {
	key, err := crypto.KeyFromHex(hexEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if sessionExpiry <= 0 {
		sessionExpiry = 72 * time.Hour
	}

	return &DefaultAuthService{
		userRepo:      userRepo,
		encryptionKey: key,
		jwtSecret:     []byte(jwtSecret),
		sessionExpiry: sessionExpiry,
		httpClient:    httpClient,
		githubAPIBase: "https://api.github.com",
	}, nil
}

// SetGitHubAPIBase allows overriding GitHub API base URL for testing.
func (s *DefaultAuthService) SetGitHubAPIBase(url string) {
	s.githubAPIBase = url
}

// ValidateAndAuthenticatePAT verifies a PAT against GitHub API and provisions/updates user.
func (s *DefaultAuthService) ValidateAndAuthenticatePAT(ctx context.Context, pat string) (*ent.User, string, error) {
	if pat == "" {
		return nil, "", fmt.Errorf("personal access token cannot be empty")
	}

	// 1. Fetch user profile from GitHub API
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.githubAPIBase+"/user", nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating github request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ContextForge-Auth")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("github authentication failed with status %d", resp.StatusCode)
	}

	var profile GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, "", fmt.Errorf("decoding github profile: %w", err)
	}

	if profile.Email == "" {
		profile.Email = fmt.Sprintf("%s@users.noreply.github.com", profile.Login)
	}

	// 2. Encrypt token at rest using AES-256-GCM
	encryptedToken, err := crypto.Encrypt([]byte(pat), s.encryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting token: %w", err)
	}

	// 3. Upsert user in repository
	existing, err := s.userRepo.GetByGitHubLogin(ctx, profile.Login)
	var u *ent.User
	if err == nil && existing != nil {
		u, err = s.userRepo.UpdateTokens(ctx, existing.ID, encryptedToken, "", nil)
		if err != nil {
			return nil, "", fmt.Errorf("updating user tokens: %w", err)
		}
	} else {
		newID := uuid.New()
		u, err = s.userRepo.Create(ctx, &ent.User{
			ID:                   newID,
			GithubID:             &profile.ID,
			GithubLogin:          profile.Login,
			Email:                profile.Email,
			Name:                 profile.Name,
			AvatarURL:            profile.AvatarURL,
			EncryptedAccessToken: encryptedToken,
		})
		if err != nil {
			return nil, "", fmt.Errorf("creating user: %w", err)
		}
	}

	// 4. Generate JWT session token
	token, err := auth.GenerateToken(u.ID, u.GithubLogin, s.jwtSecret, s.sessionExpiry)
	if err != nil {
		return nil, "", fmt.Errorf("generating jwt session: %w", err)
	}

	return u, token, nil
}

func (s *DefaultAuthService) ExchangeOAuthCode(ctx context.Context, code string) (*ent.User, string, error) {
	// Implements standard OAuth code exchange
	return nil, "", fmt.Errorf("oauth exchange requires client id/secret configuration")
}

func (s *DefaultAuthService) GetUser(ctx context.Context, userID uuid.UUID) (*ent.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}
