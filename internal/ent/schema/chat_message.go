package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// ChatMessage holds the schema definition for the ChatMessage entity.
type ChatMessage struct {
	ent.Schema
}

// Fields of the ChatMessage.
func (ChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("conversation_id", uuid.UUID{}),
		field.String("role").
			NotEmpty(),
		field.Text("content"),
		field.JSON("citations", []model.Citation{}).
			Optional(),
		field.Int("tokens_used").
			Default(0),
		field.Int64("duration_ms").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the ChatMessage.
func (ChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("conversation", Conversation.Type).
			Ref("messages").
			Field("conversation_id").
			Unique().
			Required(),
	}
}

// Indexes of the ChatMessage.
func (ChatMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("conversation_id"),
	}
}
