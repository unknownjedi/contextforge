package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"
	"github.com/your-org/contextforge/internal/ent/enttest"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

func TestEntConversationRepository(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&_pragma=foreign_keys(1)")
	defer client.Close()

	repo := repository.NewConversationRepository(client)
	ctx := context.Background()

	// Create user & project since conversations have foreign keys to project
	user, err := client.User.Create().
		SetEmail("test@example.com").
		SetGithubLogin("testuser").
		Save(ctx)
	require.NoError(t, err)

	project, err := client.Project.Create().
		SetName("Test Project").
		SetOwnerUserID(user.ID).
		Save(ctx)
	require.NoError(t, err)

	projectID := project.ID

	t.Run("CreateConversation and default title", func(t *testing.T) {
		conv, err := repo.CreateConversation(ctx, projectID, "")
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, conv.ID)
		assert.Equal(t, projectID, conv.ProjectID)
		assert.Equal(t, "New Chat", conv.Title)

		conv2, err := repo.CreateConversation(ctx, projectID, "Custom Title")
		require.NoError(t, err)
		assert.Equal(t, "Custom Title", conv2.Title)
	})

	t.Run("ListByProjectID", func(t *testing.T) {
		convs, err := repo.ListByProjectID(ctx, projectID)
		require.NoError(t, err)
		assert.Len(t, convs, 2)
	})

	t.Run("CreateMessage and ListMessages", func(t *testing.T) {
		conv, err := repo.CreateConversation(ctx, projectID, "Chat with Messages")
		require.NoError(t, err)

		citations := []model.Citation{
			{
				SourceID:   uuid.New(),
				FilePath:   "internal/crypto/encrypt.go",
				StartLine:  10,
				EndLine:    25,
				Similarity: 0.95,
				Snippet:    "func Encrypt()",
			},
		}

		userMsg, err := repo.CreateMessage(ctx, conv.ID, "user", "How does crypto work?", nil, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, "user", userMsg.Role)
		assert.Equal(t, "How does crypto work?", userMsg.Content)
		assert.Empty(t, userMsg.Citations)

		assistantMsg, err := repo.CreateMessage(ctx, conv.ID, "assistant", "It encrypts data.", citations, 25, 120)
		require.NoError(t, err)
		assert.Equal(t, "assistant", assistantMsg.Role)
		assert.Equal(t, "It encrypts data.", assistantMsg.Content)
		require.Len(t, assistantMsg.Citations, 1)
		assert.Equal(t, "internal/crypto/encrypt.go", assistantMsg.Citations[0].FilePath)
		assert.Equal(t, 25, assistantMsg.TokensUsed)
		assert.Equal(t, int64(120), assistantMsg.DurationMs)

		msgs, err := repo.ListMessages(ctx, conv.ID)
		require.NoError(t, err)
		require.Len(t, msgs, 2)
		assert.Equal(t, userMsg.ID, msgs[0].ID)
		assert.Equal(t, assistantMsg.ID, msgs[1].ID)

		// Also check GetByID eager-loads messages
		convWithMsgs, err := repo.GetByID(ctx, conv.ID, projectID)
		require.NoError(t, err)
		require.Len(t, convWithMsgs.Edges.Messages, 2)
	})

	t.Run("UpdateTitle", func(t *testing.T) {
		conv, err := repo.CreateConversation(ctx, projectID, "Initial Title")
		require.NoError(t, err)

		updated, err := repo.UpdateTitle(ctx, conv.ID, projectID, "Updated Title")
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", updated.Title)

		fetched, err := repo.GetByID(ctx, conv.ID, projectID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", fetched.Title)
	})

	t.Run("Delete", func(t *testing.T) {
		conv, err := repo.CreateConversation(ctx, projectID, "To Delete")
		require.NoError(t, err)

		_, err = repo.CreateMessage(ctx, conv.ID, "user", "msg", nil, 5, 0)
		require.NoError(t, err)

		err = repo.Delete(ctx, conv.ID, projectID)
		require.NoError(t, err)

		_, err = repo.GetByID(ctx, conv.ID, projectID)
		assert.Error(t, err)

		msgs, err := repo.ListMessages(ctx, conv.ID)
		require.NoError(t, err)
		assert.Empty(t, msgs)

		// Deleting non-existent conversation returns error
		err = repo.Delete(ctx, uuid.New(), projectID)
		assert.Error(t, err)
	})

	t.Run("Project Isolation", func(t *testing.T) {
		projectB, err := client.Project.Create().
			SetName("Project B").
			SetOwnerUserID(user.ID).
			Save(ctx)
		require.NoError(t, err)

		convA, err := repo.CreateConversation(ctx, projectID, "Project A Chat")
		require.NoError(t, err)

		convB, err := repo.CreateConversation(ctx, projectB.ID, "Project B Chat")
		require.NoError(t, err)

		// List for Project A should only contain Project A conversations
		listA, err := repo.ListByProjectID(ctx, projectID)
		require.NoError(t, err)
		for _, c := range listA {
			assert.Equal(t, projectID, c.ProjectID)
			assert.NotEqual(t, projectB.ID, c.ProjectID)
		}

		// List for Project B should only contain Project B conversation
		listB, err := repo.ListByProjectID(ctx, projectB.ID)
		require.NoError(t, err)
		require.Len(t, listB, 1)
		assert.Equal(t, convB.ID, listB[0].ID)

		// Cannot GetByID using wrong project
		_, err = repo.GetByID(ctx, convA.ID, projectB.ID)
		assert.Error(t, err)

		// Cannot UpdateTitle using wrong project
		_, err = repo.UpdateTitle(ctx, convA.ID, projectB.ID, "Hacked Title")
		assert.Error(t, err)

		// Cannot Delete using wrong project
		err = repo.Delete(ctx, convA.ID, projectB.ID)
		assert.Error(t, err)
	})
}
