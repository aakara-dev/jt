package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// BoardColumn is a column on an agile board and the statuses mapped to it.
type BoardColumn struct {
	Name     string `json:"name"`
	Statuses []struct {
		ID string `json:"id"`
	} `json:"statuses"`
}

// BoardConfig is the subset of an agile board's configuration jt uses.
type BoardConfig struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ColumnConfig struct {
		Columns []BoardColumn `json:"columns"`
	} `json:"columnConfig"`
}

// BoardConfiguration returns the column layout of a board (Agile API).
func (c *Client) BoardConfiguration(boardID int) (*BoardConfig, error) {
	var out BoardConfig
	if err := c.agileGet(fmt.Sprintf("/board/%d/configuration", boardID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BoardIssues returns issues on a board, optionally filtered by JQL. It is
// used for kanban boards, which have no sprints.
func (c *Client) BoardIssues(boardID int, jql string, from, limit uint) (*SearchResult, error) {
	path := fmt.Sprintf("/board/%d/issue?startAt=%d&maxResults=%d", boardID, from, limit)
	if jql != "" {
		path += "&jql=" + url.QueryEscape(jql)
	}
	var out SearchResult
	if err := c.agileGet(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RankIssues ranks issues before (or, if before is empty, after) another
// issue on the board.
func (c *Client) RankIssues(keys []string, before, after string) error {
	data := struct {
		Issues []string `json:"issues"`
		Before string   `json:"rankBeforeIssue,omitempty"`
		After  string   `json:"rankAfterIssue,omitempty"`
	}{keys, before, after}
	return c.agileWrite(http.MethodPut, "/issue/rank", data, http.StatusNoContent)
}

// MoveToBacklog removes issues from their sprints.
func (c *Client) MoveToBacklog(keys ...string) error {
	data := struct {
		Issues []string `json:"issues"`
	}{keys}
	return c.agileWrite(http.MethodPost, "/backlog/issue", data, http.StatusNoContent)
}

// UnwatchIssueV2 removes a watcher from an issue (Jira Server/DC).
func (c *Client) UnwatchIssueV2(key, username string) error {
	path := fmt.Sprintf("/issue/%s/watchers?username=%s", key, url.QueryEscape(username))
	res, err := c.DeleteV2(context.Background(), path, nil)
	if err != nil {
		return err
	}
	if res == nil {
		return ErrEmptyResponse
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		return formatUnexpectedResponse(res)
	}
	return nil
}

func (c *Client) agileGet(path string, out any) error {
	res, err := c.GetV1(context.Background(), path, nil)
	if err != nil {
		return err
	}
	if res == nil {
		return ErrEmptyResponse
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return formatUnexpectedResponse(res)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *Client) agileWrite(method, path string, data any, want int) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	header := Header{"Accept": "application/json", "Content-Type": "application/json"}
	var res *http.Response
	switch method {
	case http.MethodPut:
		res, err = c.PutV1(context.Background(), path, body, header)
	default:
		res, err = c.PostV1(context.Background(), path, body, header)
	}
	if err != nil {
		return err
	}
	if res == nil {
		return ErrEmptyResponse
	}
	defer func() { _ = res.Body.Close() }()
	// 207 means partial success for bulk rank; treat anything else as failure.
	if res.StatusCode != want && res.StatusCode != http.StatusMultiStatus {
		return formatUnexpectedResponse(res)
	}
	return nil
}
