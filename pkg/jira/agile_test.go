package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoardConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rest/agile/1.0/board/7/configuration", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":7,"name":"DATA board","type":"scrum","columnConfig":{"columns":[
			{"name":"To Do","statuses":[{"id":"1"}]},{"name":"Doing","statuses":[{"id":"3"},{"id":"4"}]}]}}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Server: srv.URL}, WithTimeout(3*time.Second))
	cfg, err := c.BoardConfiguration(7)
	require.NoError(t, err)
	require.Len(t, cfg.ColumnConfig.Columns, 2)
	assert.Equal(t, "Doing", cfg.ColumnConfig.Columns[1].Name)
	assert.Equal(t, "4", cfg.ColumnConfig.Columns[1].Statuses[1].ID)
}

func TestRankIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/rest/agile/1.0/issue/rank", r.URL.Path)
		var got map[string]any
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, map[string]any{"issues": []any{"DATA-2"}, "rankBeforeIssue": "DATA-1"}, got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Server: srv.URL}, WithTimeout(3*time.Second))
	require.NoError(t, c.RankIssues([]string{"DATA-2"}, "DATA-1", ""))
}

func TestMoveToBacklogError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rest/agile/1.0/backlog/issue", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorMessages":["nope"]}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Server: srv.URL}, WithTimeout(3*time.Second))
	assert.Error(t, c.MoveToBacklog("DATA-1"))
}

func TestUnwatchIssueV2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/rest/api/2/issue/DATA-1/watchers", r.URL.Path)
		assert.Equal(t, "ada", r.URL.Query().Get("username"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Server: srv.URL}, WithTimeout(3*time.Second))
	require.NoError(t, c.UnwatchIssueV2("DATA-1", "ada"))
}
