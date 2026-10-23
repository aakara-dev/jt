package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// SearchResult struct holds response from /search endpoint.
type SearchResult struct {
	StartAt       int      `json:"startAt"`
	Total         int      `json:"total"`
	IsLast        bool     `json:"isLast"`
	NextPageToken string   `json:"nextPageToken"`
	Issues        []*Issue `json:"issues"`
}

// Search searches for issues using v3 version of the Jira GET /search endpoint.
func (c *Client) Search(jql string, limit uint) (*SearchResult, error) {
	path := fmt.Sprintf("/search/jql?jql=%s&maxResults=%d&fields=*all", url.QueryEscape(jql), limit)
	return c.search(path, apiVersion3)
}

// SearchV2 searches an issues using v2 version of the Jira GET /search endpoint.
func (c *Client) SearchV2(jql string, from, limit uint) (*SearchResult, error) {
	path := fmt.Sprintf("/search?jql=%s&startAt=%d&maxResults=%d", url.QueryEscape(jql), from, limit)
	return c.search(path, apiVersion2)
}

func (c *Client) search(path, ver string) (*SearchResult, error) {
	var (
		res *http.Response
		err error
	)

	switch ver {
	case apiVersion2:
		res, err = c.GetV2(context.Background(), path, nil)
	default:
		res, err = c.Get(context.Background(), path, nil)
	}

	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrEmptyResponse
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, formatUnexpectedResponse(res)
	}

	var out SearchResult

	err = json.NewDecoder(res.Body).Decode(&out)

	return &out, err
}

// SavedFilter is a Jira saved filter.
type SavedFilter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	JQL  string `json:"jql"`
}

// FavouriteFiltersV2 returns the user's starred filters (Jira Server/DC).
func (c *Client) FavouriteFiltersV2() ([]*SavedFilter, error) {
	res, err := c.GetV2(context.Background(), "/filter/favourite", nil)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrEmptyResponse
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, formatUnexpectedResponse(res)
	}

	var out []*SavedFilter
	err = json.NewDecoder(res.Body).Decode(&out)

	return out, err
}
