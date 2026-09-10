package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Conversation holds the schema definition for the Conversation entity.
type Conversation struct {
	ent.Schema
}

// Fields of the Conversation.
func (Conversation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("title").
			Default("New Chat").
			MaxLen(255),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Conversation.
func (Conversation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("conversations").
			Field("project_id").
			Unique().
			Required(),
		edge.To("messages", ChatMessage.Type),
	}
}

// Indexes of the Conversation.
func (Conversation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
	}
}
