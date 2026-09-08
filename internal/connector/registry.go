package connector

import (
	"fmt"
	"sync"
)

// ConnectorRegistry manages registered database connectors.
type ConnectorRegistry struct {
	mu         sync.RWMutex
	connectors map[DatabaseType]DatabaseConnector
}

// NewRegistry creates a new ConnectorRegistry.
func NewRegistry() *ConnectorRegistry {
	return &ConnectorRegistry{
		connectors: make(map[DatabaseType]DatabaseConnector),
	}
}

// Register registers a connector for a specific database type.
func (r *ConnectorRegistry) Register(c DatabaseConnector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[c.Type()] = c
}

// Get retrieves a connector for the given database type.
func (r *ConnectorRegistry) Get(dbType DatabaseType) (DatabaseConnector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.connectors[dbType]
	if !ok {
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
	return c, nil
}

// SupportedTypes returns all registered database types.
func (r *ConnectorRegistry) SupportedTypes() []DatabaseType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]DatabaseType, 0, len(r.connectors))
	for t := range r.connectors {
		types = append(types, t)
	}
	return types
}

var (
	defaultRegistry     *ConnectorRegistry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the singleton registry with all built-in connectors pre-registered.
func DefaultRegistry() *ConnectorRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
		defaultRegistry.Register(NewPostgresConnector(TypePostgres))
		defaultRegistry.Register(NewPostgresConnector(TypeCockroachDB))
		defaultRegistry.Register(NewMySQLConnector(TypeMySQL))
		defaultRegistry.Register(NewMySQLConnector(TypeMariaDB))
		defaultRegistry.Register(NewSQLiteConnector())
		defaultRegistry.Register(NewSQLServerConnector())
	})
	return defaultRegistry
}
