package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestIssuesListByRepo_EmbeddedOptionsKeepPageSize(t *testing.T) {
	var paths, queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("page") == "" {
			// First request omitted page/per_page; server reports its defaults.
			w.Header().Set("Link", `<http://`+r.Host+`/repos/o/r/issues?page=2&per_page=30&state=open>; rel="next"`)
			w.Write([]byte(`[{"number":1,"title":"first"}]`))
			return
		}
		w.Write([]byte(`[{"number":2,"title":"second"}]`))
	}))
	defer srv.Close()

	client := NewClient(nil)
	client.BaseURL, _ = url.Parse(srv.URL + "/")

	opts := &IssueListByRepoOptions{State: "open"} // PerPage == 0
	var numbers []int
	for {
		issues, resp, err := client.Issues.ListByRepo(context.Background(), "o", "r", opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range issues {
			numbers = append(numbers, i.Number)
		}
		next := resp.NextListOptions()
		if next == nil {
			break
		}
		opts.ListOptions = *next
	}

	if want := []int{1, 2}; !reflect.DeepEqual(numbers, want) {
		t.Errorf("issue numbers = %v, want %v", numbers, want)
	}
	if want := []string{"/repos/o/r/issues", "/repos/o/r/issues"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
	if want := []string{"state=open", "page=2&per_page=30&state=open"}; !reflect.DeepEqual(queries, want) {
		t.Errorf("queries = %q, want %q", queries, want)
	}
}
