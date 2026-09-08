package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/user"
)

type UserRepository interface {
	Create(ctx context.Context, u *ent.User) (*ent.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ent.User, error)
	GetByGitHubLogin(ctx context.Context, login string) (*ent.User, error)
	GetByGitHubID(ctx context.Context, githubID int64) (*ent.User, error)
	UpdateTokens(ctx context.Context, id uuid.UUID, accessToken, refreshToken string, expiresAt *time.Time) (*ent.User, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type EntUserRepository struct {
	client *ent.Client
}

func NewUserRepository(client *ent.Client) *EntUserRepository {
	return &EntUserRepository{client: client}
}

func (r *EntUserRepository) Create(ctx context.Context, u *ent.User) (*ent.User, error) {
	builder := r.client.User.Create().
		SetGithubLogin(u.GithubLogin).
		SetEmail(u.Email).
		SetName(u.Name).
		SetAvatarURL(u.AvatarURL).
		SetEncryptedAccessToken(u.EncryptedAccessToken).
		SetEncryptedRefreshToken(u.EncryptedRefreshToken)

	if u.ID != uuid.Nil {
		builder.SetID(u.ID)
	}
	if u.GithubID != nil {
		builder.SetGithubID(*u.GithubID)
	}
	if u.TokenExpiresAt != nil {
		builder.SetTokenExpiresAt(*u.TokenExpiresAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return created, nil
}

func (r *EntUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*ent.User, error) {
	u, err := r.client.User.Query().Where(user.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return u, nil
}

func (r *EntUserRepository) GetByGitHubLogin(ctx context.Context, login string) (*ent.User, error) {
	u, err := r.client.User.Query().Where(user.GithubLoginEQ(login)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting user by github login: %w", err)
	}
	return u, nil
}

func (r *EntUserRepository) GetByGitHubID(ctx context.Context, githubID int64) (*ent.User, error) {
	u, err := r.client.User.Query().Where(user.GithubIDEQ(githubID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting user by github id: %w", err)
	}
	return u, nil
}

func (r *EntUserRepository) UpdateTokens(ctx context.Context, id uuid.UUID, accessToken, refreshToken string, expiresAt *time.Time) (*ent.User, error) {
	updater := r.client.User.UpdateOneID(id).
		SetEncryptedAccessToken(accessToken).
		SetEncryptedRefreshToken(refreshToken)

	if expiresAt != nil {
		updater.SetTokenExpiresAt(*expiresAt)
	} else {
		updater.ClearTokenExpiresAt()
	}

	updated, err := updater.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating user tokens: %w", err)
	}
	return updated, nil
}

func (r *EntUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.User.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	return nil
}
