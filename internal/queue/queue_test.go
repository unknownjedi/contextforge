package queue_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/contextforge/internal/queue"
)

func TestNewRepoSyncTask(t *testing.T) {
	jobID := uuid.New()
	projectID := uuid.New()
	sourceID := uuid.New()

	payload := queue.RepoSyncPayload{
		JobID:      jobID,
		ProjectID:  projectID,
		SourceID:   sourceID,
		ForceFull:  true,
		CommitHash: "commit123",
	}

	task, err := queue.NewRepoSyncTask(payload)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, queue.TypeRepoSync, task.Type())

	var decoded queue.RepoSyncPayload
	err = json.Unmarshal(task.Payload(), &decoded)
	require.NoError(t, err)

	assert.Equal(t, jobID, decoded.JobID)
	assert.Equal(t, projectID, decoded.ProjectID)
	assert.Equal(t, sourceID, decoded.SourceID)
	assert.True(t, decoded.ForceFull)
	assert.Equal(t, "commit123", decoded.CommitHash)
}

func TestNewDocEmbedTask(t *testing.T) {
	jobID := uuid.New()
	projectID := uuid.New()
	docID := uuid.New()

	payload := queue.DocEmbedPayload{
		JobID:      jobID,
		ProjectID:  projectID,
		DocumentID: docID,
	}

	task, err := queue.NewDocEmbedTask(payload)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, queue.TypeDocEmbed, task.Type())

	var decoded queue.DocEmbedPayload
	err = json.Unmarshal(task.Payload(), &decoded)
	require.NoError(t, err)

	assert.Equal(t, jobID, decoded.JobID)
	assert.Equal(t, projectID, decoded.ProjectID)
	assert.Equal(t, docID, decoded.DocumentID)
}

func TestNewClient_InvalidRedisURL(t *testing.T) {
	client, err := queue.NewClient("invalid://url")
	assert.Error(t, err)
	assert.Nil(t, client)
}
