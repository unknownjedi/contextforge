package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Source holds the schema definition for the Source entity.
type Source struct {
	ent.Schema
}

// Fields of the Source.
func (Source) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.String("name").
			NotEmpty(),
		field.String("type").
			Default("github"),
		field.String("repo_owner").
			NotEmpty(),
		field.String("repo_name").
			NotEmpty(),
		field.String("branch").
			Default("main"),
		field.String("last_commit_hash").
			Optional().
			Default(""),
		field.Enum("sync_status").
			Values("idle", "queued", "syncing", "synced", "failed").
			Default("idle"),
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

// Edges of the Source.
func (Source) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("sources").
			Field("project_id").
			Unique().
			Required(),
		edge.To("documents", Document.Type),
		edge.To("jobs", IngestionJob.Type),
	}
}

// Indexes of the Source.
func (Source) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("project_id", "repo_owner", "repo_name", "branch").Unique(),
	}
}
