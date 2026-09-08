package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// IngestionJob holds the schema definition for the IngestionJob entity.
type IngestionJob struct {
	ent.Schema
}

// Fields of the IngestionJob.
func (IngestionJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("source_id", uuid.UUID{}),
		field.Enum("status").
			Values("pending", "running", "completed", "failed").
			Default("pending"),
		field.Int("progress_percent").
			Default(0),
		field.Int("processed_files").
			Default(0),
		field.Int("total_files").
			Default(0),
		field.String("error_message").
			Optional().
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.Time("finished_at").
			Optional().
			Nillable(),
	}
}

// Edges of the IngestionJob.
func (IngestionJob) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("jobs").
			Field("project_id").
			Unique().
			Required(),
		edge.From("source", Source.Type).
			Ref("jobs").
			Field("source_id").
			Unique().
			Required(),
	}
}

// Indexes of the IngestionJob.
func (IngestionJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("source_id"),
		index.Fields("status"),
	}
}
