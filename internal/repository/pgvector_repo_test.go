package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

func TestFormatVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected string
	}{
		{
			name:     "empty slice",
			input:    []float32{},
			expected: "[]",
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: "[]",
		},
		{
			name:     "single element",
			input:    []float32{0.5},
			expected: "[0.5]",
		},
		{
			name:     "multiple elements",
			input:    []float32{0.1, 0.25, -0.75},
			expected: "[0.1,0.25,-0.75]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repository.FormatVector(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPgVectorRepository_ValidationErrors(t *testing.T) {
	repo := repository.NewPgVectorRepository(nil)
	ctx := context.Background()

	t.Run("Requires ProjectID", func(t *testing.T) {
		_, err := repo.SearchSimilar(ctx, model.VectorSearchParams{
			ProjectID:      uuid.Nil,
			QueryEmbedding: []float32{0.1, 0.2},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project ID is required")
	})

	t.Run("Requires QueryEmbedding", func(t *testing.T) {
		_, err := repo.SearchSimilar(ctx, model.VectorSearchParams{
			ProjectID:      uuid.New(),
			QueryEmbedding: nil,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "query embedding cannot be empty")
	})
}
