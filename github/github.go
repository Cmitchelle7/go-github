// Package github is a minimal client skeleton that demonstrates consistent
// pagination handling, including the ListOptions.PerPage == 0 case.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-querystring/query"
)

// ListOptions specifies the optional parameters to various List methods that
// support offset pagination.
//
// A zero Page or PerPage is omitted from the query string, so the server's
// defaults apply (page 1, and typically 30 items per page for GitHub). The
// Link header the server returns always carries explicit page and per_page
// values, which Response exposes so follow-up requests can keep the same page
// size (see Response.NextListOptions).
type ListOptions struct {
	// For paginated result sets, the page of results to retrieve.
	Page int `url:"page,omitempty"`

	// For paginated result sets, the number of results to include per page.
	// If 0, the server default is used.
	PerPage int `url:"per_page,omitempty"`
}

// Response wraps http.Response and adds pagination information parsed from
// the Link header.
type Response struct {
	*http.Response

	// Page numbers extracted from the Link header. A value of 0 means the
	// relation was not present (e.g. NextPage == 0 on the last page).
	NextPage  int
	PrevPage  int
	FirstPage int
	LastPage  int

	// PerPage is the page size the server reported in the Link header, even
	// when the request itself omitted per_page. It is 0 if no Link header (or
	// no per_page value) was present.
	PerPage int
}

func newResponse(r *http.Response) *Response {
	resp := &Response{Response: r}
	resp.parsePagination()
	return resp
}

// parsePagination fills the page fields from the Link header. Malformed
// segments are skipped rather than treated as errors.
func (r *Response) parsePagination() {
	if r.Response == nil {
		return
	}
	for _, header := range r.Header.Values("Link") {
		for _, segment := range strings.Split(header, ",") {
			parts := strings.Split(strings.TrimSpace(segment), ";")
			if len(parts) < 2 {
				continue
			}
			target := strings.TrimSpace(parts[0])
			if len(target) < 2 || target[0] != '<' || target[len(target)-1] != '>' {
				continue
			}
			u, err := url.Parse(target[1 : len(target)-1])
			if err != nil {
				continue
			}
			q := u.Query()
			page, _ := strconv.Atoi(q.Get("page"))
			perPage, _ := strconv.Atoi(q.Get("per_page"))

			var rels []string
			for _, p := range parts[1:] {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "rel=") {
					rels = strings.Fields(strings.Trim(strings.TrimPrefix(p, "rel="), `"`))
				}
			}
			for _, rel := range rels {
				switch rel {
				case "first":
					r.FirstPage = page
				case "prev":
					r.PrevPage = page
				case "next":
					r.NextPage = page
				case "last":
					r.LastPage = page
				}
			}
			if perPage > 0 && r.PerPage == 0 {
				r.PerPage = perPage
			}
		}
	}
}

// NextListOptions returns options for the next page that keep the page size
// the server used, or nil if there is no next page.
func (r *Response) NextListOptions() *ListOptions {
	if r.NextPage == 0 {
		return nil
	}
	return &ListOptions{Page: r.NextPage, PerPage: r.PerPage}
}

// addOptions encodes opts (a struct with `url` tags, possibly embedding
// ListOptions) into the query string of s. Query parameters already present in
// s are kept; parameters from opts override any with the same name.
func addOptions(s string, opts interface{}) (string, error) {
	if opts == nil {
		return s, nil
	}
	qs, err := query.Values(opts)
	if err != nil {
		return s, err
	}
	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}
	merged := u.Query()
	for k, v := range qs {
		merged[k] = v
	}
	u.RawQuery = merged.Encode()
	return u.String(), nil
}

// Client is a minimal API client.
type Client struct {
	client  *http.Client
	BaseURL *url.URL // must have a trailing slash

	Repositories *RepositoriesService
	Issues       *IssuesService
}

// NewClient returns a Client pointed at the public GitHub API.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	base, _ := url.Parse("https://api.github.com/")
	c := &Client{client: httpClient, BaseURL: base}
	c.Repositories = &RepositoriesService{client: c}
	c.Issues = &IssuesService{client: c}
	return c
}

// NewRequest creates a request for a path relative to BaseURL.
func (c *Client) NewRequest(method, urlStr string) (*http.Request, error) {
	if !strings.HasSuffix(c.BaseURL.Path, "/") {
		return nil, fmt.Errorf("BaseURL must have a trailing slash, but %q does not", c.BaseURL)
	}
	u, err := c.BaseURL.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return http.NewRequest(method, u.String(), nil)
}

// Do sends the request, decodes a JSON body into v, and returns a Response
// with pagination fields populated.
func (c *Client) Do(ctx context.Context, req *http.Request, v interface{}) (*Response, error) {
	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	response := newResponse(resp)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return response, fmt.Errorf("%s %s: %s", req.Method, req.URL, resp.Status)
	}
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return response, err
		}
	}
	return response, nil
}

// RepositoriesService handles repository endpoints.
type RepositoriesService struct{ client *Client }

// Repository is a trimmed-down repository resource.
type Repository struct {
	Name string `json:"name"`
}

// List lists repositories for the authenticated user.
func (s *RepositoriesService) List(ctx context.Context, opts *ListOptions) ([]*Repository, *Response, error) {
	path := "user/repos"
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
	var repos []*Repository
	resp, err := s.client.Do(ctx, req, &repos)
	if err != nil {
		return nil, resp, err
	}
	return repos, resp, nil
}
