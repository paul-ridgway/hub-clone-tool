package hct

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNextLink(t *testing.T) {
	tests := map[string]string{
		"": "",
		`<https://api.github.com/user/repos?page=1>; rel="prev"`:                                                         "",
		`<https://api.github.com/user/repos?page=2>; rel="next", <https://api.github.com/user/repos?page=5>; rel="last"`: "https://api.github.com/user/repos?page=2",
		`<https://api.github.com/a?page=1>; rel="prev", <https://api.github.com/a?page=3>; rel="next"`:                   "https://api.github.com/a?page=3",
	}
	for header, want := range tests {
		if got := nextLink(header); got != want {
			t.Errorf("nextLink(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestPathForRepo(t *testing.T) {
	got := pathForRepo("/code", repo{org: " MyOrg ", name: "Thing"})
	if want := "/code/myorg/Thing"; got != want {
		t.Errorf("pathForRepo = %q, want %q", got, want)
	}
}

func TestListOrgReposPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `[{"name":"b","ssh_url":"git@github.com:acme/b.git","archived":true}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/acme/repos?page=2>; rel="next"`, srv.URL))
		fmt.Fprint(w, `[{"name":"a","ssh_url":"git@github.com:acme/a.git"}]`)
	}))
	defer srv.Close()

	c := &client{token: "tok", baseURL: srv.URL, http: srv.Client()}
	repos, err := c.listOrgRepos(context.Background(), "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []repo{
		{org: "acme", name: "a", gitURL: "git@github.com:acme/a.git"},
		{org: "acme", name: "b", gitURL: "git@github.com:acme/b.git", archived: true},
	}
	if len(repos) != len(want) || repos[0] != want[0] || repos[1] != want[1] {
		t.Errorf("repos = %+v, want %+v", repos, want)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	}))
	defer srv.Close()

	c := &client{token: "tok", baseURL: srv.URL, http: srv.Client()}
	if _, err := c.listOrgs(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestProcessRepos(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	for _, args := range [][]string{
		{"init", "-q", src},
		{"-C", src, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "acme", "existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	repos := []repo{
		{org: "Acme", name: "new", gitURL: src},
		{org: "Acme", name: "existing", gitURL: src},
		{org: "Acme", name: "broken", gitURL: filepath.Join(base, "does-not-exist")},
	}

	got := processRepos(context.Background(), base, repos, 2)
	if got.cloned != 1 || got.skipped != 1 || len(got.failures) != 1 {
		t.Errorf("stats = %+v, want 1 cloned, 1 skipped, 1 failure", got)
	}
	if !exists(filepath.Join(base, "acme", "new", ".git")) {
		t.Error("repo was not cloned")
	}
}

func TestParseArgs(t *testing.T) {
	o, _, ok := parseArgs([]string{"hct", "--dir", "/a", "-c", "/b"})
	if !ok || o.dir != "/a" || o.config != "/b" {
		t.Errorf("parseArgs = %+v, ok=%v", o, ok)
	}
	if _, code, ok := parseArgs([]string{"hct", "--help"}); ok || code != 0 {
		t.Errorf("--help: code=%d ok=%v", code, ok)
	}
	if _, code, ok := parseArgs([]string{"hct", "--nope"}); ok || code != 2 {
		t.Errorf("unknown flag: code=%d ok=%v", code, ok)
	}
}

func TestGitConfigFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(file, []byte("[github]\n\tapikey = abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := gitConfig(file, "github.apikey"); got != "abc" {
		t.Errorf("apikey = %q", got)
	}
	if got := gitConfig(file, "code.home", "--type=path"); got != "" {
		t.Errorf("code.home = %q", got)
	}
}

func TestProgressKeepsLastFinished(t *testing.T) {
	p := &progress{tty: true, slots: make([]slot, 1)}
	for i := 0; i < maxRecent+5; i++ {
		p.finish(0, fmt.Sprintf("repo %d", i))
	}
	if len(p.recent) != maxRecent || p.recent[0] != "repo 5" || p.recent[maxRecent-1] != "repo 14" {
		t.Errorf("recent = %v", p.recent)
	}
	p.redraw()
	if p.drawn != maxRecent {
		t.Errorf("drawn = %d, want %d", p.drawn, maxRecent)
	}
}

func TestProgressLayoutFitsTerminal(t *testing.T) {
	rows := 0
	p := &progress{tty: true, slots: make([]slot, 4), rows: func() int { return rows }}
	for i := range p.slots {
		p.slots[i] = slot{text: fmt.Sprintf("repo %d", i), detail: "Receiving objects", active: true}
	}
	p.recent = []string{"a", "b", "c"}

	tests := []struct {
		rows      int
		wantLines int
		wantLast  string
	}{
		{rows: 30, wantLines: 11, wantLast: "a"},            // everything, two lines per slot
		{rows: 10, wantLines: 9, wantLast: "c"},             // drops older recent lines
		{rows: 6, wantLines: 5, wantLast: "c"},              // details move inline
		{rows: 4, wantLines: 3, wantLast: "  … and 2 more"}, // slots summarised
	}
	for _, tt := range tests {
		rows = tt.rows
		lines := p.layout()
		if len(lines) != tt.wantLines || lines[0][:len("⠋ repo 0")] != "⠋ repo 0" || lines[len(lines)-1] != tt.wantLast {
			t.Errorf("rows=%d: got %d lines ending %q, want %d ending %q\n%q", tt.rows, len(lines), lines[len(lines)-1], tt.wantLines, tt.wantLast, lines)
		}
	}
}

func TestStatusBar(t *testing.T) {
	tl := &tally{total: 10, started: time.Now()}
	tl.update(func(s *stats) {
		s.cloned, s.skipped = 3, 1
		s.failures = []string{"a / b: boom"}
	})
	got := tl.statusBar()
	want := strings.Repeat("█", 15) + strings.Repeat("░", 15) + " 5/10 (50%) · 3 cloned · 1 skipped · 1 failed · 0s"
	if got != want {
		t.Errorf("statusBar = %q, want %q", got, want)
	}
}

func TestProgressFooterReservesRow(t *testing.T) {
	p := &progress{tty: true, slots: make([]slot, 1), rows: func() int { return 5 }, footer: func() string { return "footer" }}
	p.slots[0] = slot{text: "repo", detail: "Receiving", active: true}
	p.recent = []string{"a", "b", "c"}
	if lines := p.layout(); len(lines) != 3 || lines[2] != "c" {
		t.Errorf("layout = %q", lines)
	}
	p.redraw()
	if p.anchored != 5 {
		t.Errorf("anchored = %d, want 5", p.anchored)
	}
}
