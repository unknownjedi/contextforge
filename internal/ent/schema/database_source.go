package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DatabaseSource holds the schema definition for the DatabaseSource entity.
type DatabaseSource struct {
	ent.Schema
}

// Fields of the DatabaseSource.
func (DatabaseSource) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("source_id", uuid.UUID{}).
			Unique(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("database_type").
			NotEmpty(),
		field.String("host").
			Default(""),
		field.Int("port").
			Default(0),
		field.String("database_name").
			Default(""),
		field.String("username").
			Default(""),
		field.String("encrypted_connection_url").
			NotEmpty().
			Sensitive(),
		field.JSON("configuration", map[string]any{}).
			Optional(),
		field.String("status").
			Default("created"),
		field.String("last_error").
			Optional().
			Default(""),
		field.Time("last_synced_at").
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

// Edges of the DatabaseSource.
func (DatabaseSource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source", Source.Type).
			Ref("database_source").
			Field("source_id").
			Unique().
			Required(),
		edge.From("project", Project.Type).
			Ref("database_sources").
			Field("project_id").
			Unique().
			Required(),
	}
}

// Indexes of the DatabaseSource.
func (DatabaseSource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("source_id"),
	}
}
