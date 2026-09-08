package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.Int64("github_id").
			Unique().
			Optional().
			Nillable(),
		field.String("github_login").
			NotEmpty().
			Unique(),
		field.String("email").
			NotEmpty(),
		field.String("name").
			Optional().
			Default(""),
		field.String("avatar_url").
			Optional().
			Default(""),
		field.String("encrypted_access_token").
			Optional().
			Sensitive(),
		field.String("encrypted_refresh_token").
			Optional().
			Sensitive(),
		field.Time("token_expires_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("projects", Project.Type),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("github_login"),
		index.Fields("email"),
	}
}
