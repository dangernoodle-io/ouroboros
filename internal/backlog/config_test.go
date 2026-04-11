package backlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"dangernoodle.io/ouroboros/internal/backlog"
)

func TestSetAndGetConfig(t *testing.T) {
	d := testDB(t)

	err := backlog.SetConfig(d, "test-key", "test-value")
	require.NoError(t, err)

	value, err := backlog.GetConfig(d, "test-key")
	require.NoError(t, err)

	assert.Equal(t, "test-value", value)
}

func TestGetConfigNotFound(t *testing.T) {
	d := testDB(t)

	_, err := backlog.GetConfig(d, "nonexistent")
	assert.Error(t, err)
}

func TestGetAllConfig(t *testing.T) {
	d := testDB(t)

	err := backlog.SetConfig(d, "key1", "value1")
	require.NoError(t, err)

	err = backlog.SetConfig(d, "key2", "value2")
	require.NoError(t, err)

	config, err := backlog.GetAllConfig(d)
	require.NoError(t, err)

	assert.Equal(t, 2, len(config))
	assert.Equal(t, "value1", config["key1"])
	assert.Equal(t, "value2", config["key2"])
}

func TestSetConfigOverwrite(t *testing.T) {
	d := testDB(t)

	err := backlog.SetConfig(d, "test-key", "old-value")
	require.NoError(t, err)

	err = backlog.SetConfig(d, "test-key", "new-value")
	require.NoError(t, err)

	value, err := backlog.GetConfig(d, "test-key")
	require.NoError(t, err)

	assert.Equal(t, "new-value", value)
}
