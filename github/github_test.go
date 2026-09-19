package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestAddOptions(t *testing.T) {
	type issueOpts struct {
		State string `url:"state,omitempty"`
		ListOptions
	}
	tests := []struct {
		name string
		in   string
		opts interface{}
		want string
	}{
		{"nil opts", "/x", nil, "/x"},
		{"nil pointer", "/x", (*ListOptions)(nil), "/x"},
		{"zero value omits everything", "/x", &ListOptions{}, "/x"},
		{"page 1, per_page 0 omits per_page", "/x", ListOptions{Page: 1, PerPage: 0}, "/x?page=1"},
		{"page 2, per_page 50", "/x", ListOptions{Page: 2, PerPage: 50}, "/x?page=2&per_page=50"},
		{"existing query kept", "/x?state=open", ListOptions{Page: 2}, "/x?page=2&state=open"},
		{"opts override existing", "/x?page=1&per_page=5", ListOptions{Page: 3}, "/x?page=3&per_page=5"},
		{"embedded ListOptions", "/x", issueOpts{State: "open", ListOptions: ListOptions{Page: 2, PerPage: 50}}, "/x?page=2&per_page=50&state=open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addOptions(tt.in, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("addOptions() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   Response
	}{
		{"no header", "", Response{}},
		{
			"server default page size (request omitted per_page)",
			`<https://api.github.com/user/repos?page=2&per_page=30>; rel="next", <https://api.github.com/user/repos?page=5&per_page=30>; rel="last"`,
			Response{NextPage: 2, LastPage: 5, PerPage: 30},
		},
		{
			"explicit page size, middle page",
			`<https://api.github.com/user/repos?page=1&per_page=50>; rel="prev", <https://api.github.com/user/repos?page=3&per_page=50>; rel="next", <https://api.github.com/user/repos?page=4&per_page=50>; rel="last", <https://api.github.com/user/repos?page=1&per_page=50>; rel="first"`,
			Response{FirstPage: 1, PrevPage: 1, NextPage: 3, LastPage: 4, PerPage: 50},
		},
		{
			"links without per_page",
			`<https://api.github.com/user/repos?page=2>; rel="next"`,
			Response{NextPage: 2},
		},
		{
			"malformed segments are skipped",
			`garbage, <://bad url>; rel="next", <https://api.github.com/x?page=7&per_page=10>; rel="last"`,
			Response{LastPage: 7, PerPage: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			if tt.header != "" {
				h.Set("Link", tt.header)
			}
			got := newResponse(&http.Response{Header: h})
			got.Response = nil // compare only the pagination fields
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestNextListOptions(t *testing.T) {
	last := &Response{NextPage: 0, PerPage: 30}
	if last.NextListOptions() != nil {
		t.Error("expected nil on the last page")
	}
	r := &Response{NextPage: 2, PerPage: 30}
	if got := r.NextListOptions(); *got != (ListOptions{Page: 2, PerPage: 30}) {
		t.Errorf("got %+v", *got)
	}
}

// newPagedServer serves the given names, defaulting to serverDefault items per
// page when per_page is absent, and always emitting explicit page/per_page in
// the Link header, as the real API does. It records each request's RawQuery.
func newPagedServer(names []string, serverDefault int, queries *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*queries = append(*queries, r.URL.RawQuery)
		atoi := func(key string, def int) int {
			if n, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil && n > 0 {
				return n
			}
			return def
		}
		page, per := atoi("page", 1), atoi("per_page", serverDefault)
		last := (len(names) + per - 1) / per

		start := (page - 1) * per
		if start > len(names) {
			start = len(names)
		}
		end := start + per
		if end > len(names) {
			end = len(names)
		}

		link := func(p int, rel string) string {
			return fmt.Sprintf(`<http://%s/user/repos?page=%d&per_page=%d>; rel="%s"`, r.Host, p, per, rel)
		}
		var links []string
		if page < last {
			links = append(links, link(page+1, "next"), link(last, "last"))
		}
		if page > 1 {
			links = append(links, link(page-1, "prev"), link(1, "first"))
		}
		if len(links) > 0 {
			w.Header().Set("Link", strings.Join(links, ", "))
		}

		var out []map[string]string
		for _, n := range names[start:end] {
			out = append(out, map[string]string{"name": n})
		}
		json.NewEncoder(w).Encode(out)
	}))
}

func TestRepositoriesList_FollowNextPage(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e"}
	tests := []struct {
		name        string
		first       *ListOptions
		wantQueries []string
	}{
		{"nil options, server default page size", nil, []string{"", "page=2&per_page=2", "page=3&per_page=2"}},
		{"zero PerPage, server default page size", &ListOptions{Page: 0, PerPage: 0}, []string{"", "page=2&per_page=2", "page=3&per_page=2"}},
		{"explicit PerPage is preserved", &ListOptions{PerPage: 3}, []string{"per_page=3", "page=2&per_page=3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var queries []string
			srv := newPagedServer(names, 2, &queries)
			defer srv.Close()

			client := NewClient(nil)
			client.BaseURL, _ = url.Parse(srv.URL + "/")

			var got []string
			opts := tt.first
			for {
				repos, resp, err := client.Repositories.List(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range repos {
					got = append(got, r.Name)
				}
				if opts = resp.NextListOptions(); opts == nil {
					break
				}
			}
			if !reflect.DeepEqual(got, names) {
				t.Errorf("collected %v, want %v", got, names)
			}
			if !reflect.DeepEqual(queries, tt.wantQueries) {
				t.Errorf("request queries = %q, want %q", queries, tt.wantQueries)
			}
		})
	}
}
