package github

import (
	"context"
	"fmt"
)

// IssuesService handles issue endpoints.
type IssuesService struct{ client *Client }

// Issue is a trimmed-down issue resource.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// IssueListByRepoOptions specifies the optional parameters to
// IssuesService.ListByRepo. It embeds ListOptions, so the pagination rules
// documented there (zero values are omitted) apply unchanged.
type IssueListByRepoOptions struct {
	State string `url:"state,omitempty"`

	ListOptions
}

// ListByRepo lists the issues for a repository.
func (s *IssuesService) ListByRepo(ctx context.Context, owner, repo string, opts *IssueListByRepoOptions) ([]*Issue, *Response, error) {
	path := fmt.Sprintf("repos/%v/%v/issues", owner, repo)
	if opts != nil {
		var err error
		if path, err = addOptions(path, opts); err != nil {
			return nil, nil, err
		}
	}
	req, err := s.client.NewRequest("GET", path)
	if err != nil {
		return nil, nil, err
	}
	var issues []*Issue
	resp, err := s.client.Do(ctx, req, &issues)
	if err != nil {
		return nil, resp, err
	}
	return issues, resp, nil
}
