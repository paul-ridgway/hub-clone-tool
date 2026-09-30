package hct

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const githubAPI = "https://api.github.com"

type repo struct {
	org      string
	name     string
	gitURL   string
	archived bool
}

type apiOrg struct {
	Login string `json:"login"`
}

type apiRepo struct {
	Name     string `json:"name"`
	SSHURL   string `json:"ssh_url"`
	Archived bool   `json:"archived"`
}

type client struct {
	token   string
	baseURL string
	http    *http.Client
}

func newClient(token string) *client {
	return &client{token: token, baseURL: githubAPI, http: http.DefaultClient}
}

// getAll fetches every page of a list endpoint, reporting the running total to
// onPage (if not nil) after each page.
func getAll[T any](ctx context.Context, c *client, path string, onPage func(int)) ([]T, error) {
	var all []T
	next := c.baseURL + path
	for next != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "hub-clone-tool")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			var apiErr struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &apiErr)
			return nil, fmt.Errorf("GitHub API %s: %s", resp.Status, apiErr.Message)
		}

		var page []T
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if onPage != nil {
			onPage(len(all))
		}
		next = nextLink(resp.Header.Get("Link"))
	}
	return all, nil
}

// nextLink extracts the rel="next" URL from a Link header, or "" if there is none.
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(part, ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(target), "<>")
	}
	return ""
}

func (c *client) listOrgs(ctx context.Context) ([]string, error) {
	orgs, err := getAll[apiOrg](ctx, c, "/user/orgs?per_page=100", nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(orgs))
	for i, o := range orgs {
		names[i] = o.Login
	}
	return names, nil
}

func (c *client) listMyRepos(ctx context.Context, onPage func(int)) ([]repo, error) {
	data, err := getAll[apiRepo](ctx, c, "/user/repos?type=owner&per_page=100", onPage)
	if err != nil {
		return nil, err
	}
	return toRepos("personal", data), nil // TODO: Param?
}

func (c *client) listOrgRepos(ctx context.Context, org string, onPage func(int)) ([]repo, error) {
	data, err := getAll[apiRepo](ctx, c, "/orgs/"+url.PathEscape(org)+"/repos?type=all&per_page=100", onPage)
	if err != nil {
		return nil, err
	}
	return toRepos(org, data), nil
}

func toRepos(org string, data []apiRepo) []repo {
	repos := make([]repo, len(data))
	for i, r := range data {
		repos[i] = repo{org: org, name: r.Name, gitURL: r.SSHURL, archived: r.Archived}
	}
	return repos
}
