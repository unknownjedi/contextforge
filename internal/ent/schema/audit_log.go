package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// AuditLog holds the schema definition for the AuditLog entity.
type AuditLog struct {
	ent.Schema
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("user_id", uuid.UUID{}),
		field.String("action").
			NotEmpty(),
		field.String("resource_type").
			NotEmpty(),
		field.String("resource_id").
			NotEmpty(),
		field.Text("details").
			Optional().
			Default("{}"),
		field.String("ip_address").
			Optional().
			Default(""),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the AuditLog.
func (AuditLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("audit_logs").
			Field("project_id").
			Unique().
			Required(),
	}
}

// Indexes of the AuditLog.
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id"),
		index.Fields("user_id"),
		index.Fields("action"),
		index.Fields("created_at"),
	}
}
