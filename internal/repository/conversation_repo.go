package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/chatmessage"
	"github.com/your-org/contextforge/internal/ent/conversation"
	"github.com/your-org/contextforge/internal/model"
)

// ConversationRepository defines the contract for persisting conversations and messages.
type ConversationRepository interface {
	CreateConversation(ctx context.Context, projectID uuid.UUID, title string) (*ent.Conversation, error)
	ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Conversation, error)
	GetByID(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) (*ent.Conversation, error)
	UpdateTitle(ctx context.Context, convID uuid.UUID, projectID uuid.UUID, title string) (*ent.Conversation, error)
	Delete(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) error
	CreateMessage(ctx context.Context, convID uuid.UUID, role, content string, citations []model.Citation, tokensUsed int, durationMs int64) (*ent.ChatMessage, error)
	ListMessages(ctx context.Context, convID uuid.UUID) ([]*ent.ChatMessage, error)
}

// EntConversationRepository is the Ent-backed implementation of ConversationRepository.
type EntConversationRepository struct {
	client *ent.Client
}

// NewConversationRepository constructs an EntConversationRepository.
func NewConversationRepository(client *ent.Client) *EntConversationRepository {
	return &EntConversationRepository{client: client}
}

// CreateConversation creates a new conversation for a project.
func (r *EntConversationRepository) CreateConversation(ctx context.Context, projectID uuid.UUID, title string) (*ent.Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New Chat"
	}

	conv, err := r.client.Conversation.Create().
		SetProjectID(projectID).
		SetTitle(title).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating conversation: %w", err)
	}
	return conv, nil
}

// ListByProjectID returns all conversations for a given project, sorted by updated_at descending.
func (r *EntConversationRepository) ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Conversation, error) {
	convs, err := r.client.Conversation.Query().
		Where(conversation.ProjectIDEQ(projectID)).
		Order(ent.Desc(conversation.FieldUpdatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing conversations: %w", err)
	}
	return convs, nil
}

// GetByID retrieves a conversation by its ID and project ID, eager-loading its messages in chronological order.
func (r *EntConversationRepository) GetByID(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) (*ent.Conversation, error) {
	conv, err := r.client.Conversation.Query().
		Where(
			conversation.IDEQ(convID),
			conversation.ProjectIDEQ(projectID),
		).
		WithMessages(func(q *ent.ChatMessageQuery) {
			q.Order(ent.Asc(chatmessage.FieldCreatedAt))
		}).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting conversation: %w", err)
	}
	return conv, nil
}

// UpdateTitle updates the title of a conversation.
func (r *EntConversationRepository) UpdateTitle(ctx context.Context, convID uuid.UUID, projectID uuid.UUID, title string) (*ent.Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New Chat"
	}

	updated, err := r.client.Conversation.UpdateOneID(convID).
		Where(conversation.ProjectIDEQ(projectID)).
		SetTitle(title).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating conversation title: %w", err)
	}
	return updated, nil
}

// Delete deletes a conversation and cascades to its messages.
func (r *EntConversationRepository) Delete(ctx context.Context, convID uuid.UUID, projectID uuid.UUID) error {
	exists, err := r.client.Conversation.Query().
		Where(
			conversation.IDEQ(convID),
			conversation.ProjectIDEQ(projectID),
		).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("checking conversation: %w", err)
	}
	if !exists {
		return fmt.Errorf("conversation not found or unauthorized")
	}

	// Delete associated messages
	_, err = r.client.ChatMessage.Delete().
		Where(chatmessage.ConversationIDEQ(convID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting chat messages: %w", err)
	}

	deletedCount, err := r.client.Conversation.Delete().
		Where(
			conversation.IDEQ(convID),
			conversation.ProjectIDEQ(projectID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting conversation: %w", err)
	}
	if deletedCount == 0 {
		return fmt.Errorf("conversation not found or unauthorized")
	}
	return nil
}

// CreateMessage creates a new chat message associated with a conversation and updates the conversation updated_at timestamp.
func (r *EntConversationRepository) CreateMessage(ctx context.Context, convID uuid.UUID, role, content string, citations []model.Citation, tokensUsed int, durationMs int64) (*ent.ChatMessage, error) {
	if citations == nil {
		citations = []model.Citation{}
	}

	msg, err := r.client.ChatMessage.Create().
		SetConversationID(convID).
		SetRole(role).
		SetContent(content).
		SetCitations(citations).
		SetTokensUsed(tokensUsed).
		SetDurationMs(durationMs).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating chat message: %w", err)
	}

	// Update conversation's updated_at timestamp
	_ = r.client.Conversation.UpdateOneID(convID).
		SetUpdatedAt(time.Now()).
		Exec(ctx)

	return msg, nil
}

// ListMessages retrieves all messages for a conversation ordered chronologically (created_at ascending).
func (r *EntConversationRepository) ListMessages(ctx context.Context, convID uuid.UUID) ([]*ent.ChatMessage, error) {
	messages, err := r.client.ChatMessage.Query().
		Where(chatmessage.ConversationIDEQ(convID)).
		Order(ent.Asc(chatmessage.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing chat messages: %w", err)
	}
	return messages, nil
}
