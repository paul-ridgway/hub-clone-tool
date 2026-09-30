// Package hct implements the hub-clone-tool CLI.
package hct

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
)

const usage = `Clones all repositories you have access to from GitHub.

Usage:
  %s [options]

Options:
  -d, --dir <path>     Directory to clone into (default: the code.home git
                       config value, or the current directory if unset)
  -c, --config <file>  Git config file to read github.apikey and code.home
                       from (default: your global git config)
  -h, --help           Show this help

Repositories are cloned over SSH into <dir>/<org>/<repo>; personal
repositories go into <dir>/personal. Existing directories are skipped.

Configuration:
  git config --global --add github.apikey <token>   (scopes: repo, read:org)
  git config --global --add code.home <path>
`

type options struct {
	dir    string
	config string
}

// parseArgs returns the parsed options, or an exit code if the process should
// stop (help requested or invalid arguments).
func parseArgs(args []string) (options, int, bool) {
	var o options
	name := filepath.Base(args[0])
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintf(fs.Output(), usage, name) }
	fs.StringVar(&o.dir, "dir", "", "")
	fs.StringVar(&o.dir, "d", "", "")
	fs.StringVar(&o.config, "config", "", "")
	fs.StringVar(&o.config, "c", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, 0, false
		}
		return o, 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "unexpected argument: %s\n", fs.Arg(0))
		fs.Usage()
		return o, 2, false
	}
	return o, 0, true
}

// gitConfig reads a git config value from the given file, or the global config
// if file is empty, returning "" if it is not set.
func gitConfig(file, key string, extraArgs ...string) string {
	source := []string{"--global"}
	if file != "" {
		source = []string{"--file", file}
	}
	args := append(append([]string{"config"}, source...), extraArgs...)
	out, err := exec.Command("git", append(args, "--get", key)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sanitizeOrg(org string) string {
	return strings.ToLower(strings.TrimSpace(org))
}

func pathForRepo(base string, r repo) string {
	return filepath.Join(base, sanitizeOrg(r.org), r.name)
}

// basePath resolves the directory to clone into: --dir if given, then
// code.home if configured, otherwise the current directory.
func basePath(o options) (string, error) {
	if o.dir != "" {
		dir, err := filepath.Abs(o.dir)
		if err != nil {
			return "", err
		}
		if !exists(dir) {
			return "", fmt.Errorf("The given directory does not exist: %s", dir)
		}
		info("Cloning to: %s", dir)
		return dir, nil
	}
	home := gitConfig(o.config, "code.home", "--type=path")
	if home == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		warn("No directory is configured as a git global config variable (code.home).")
		warn("Working out of the current directory: %s", cwd)
		return cwd, nil
	}
	if !exists(home) {
		return "", fmt.Errorf("The configured code path does not exist: %s", home)
	}
	info("Cloning to: %s", home)
	return home, nil
}

// fetchConcurrency caps simultaneous listings to stay clear of GitHub's
// secondary rate limits.
const fetchConcurrency = 8

// fetchRepos lists the user's own repositories and those of every org in
// parallel, with a live status line for each. A failed org is reported and
// skipped; failing to list the user's own repositories is an error.
func fetchRepos(ctx context.Context, c *client, orgs []string) ([]repo, error) {
	const personal = "personal" // TODO: Param?
	labels := append([]string{personal}, orgs...)
	found := make([][]repo, len(labels))
	var personalErr error

	p := newProgress(len(labels))
	defer p.close()
	for i, label := range labels {
		p.set(i, label+": waiting...")
	}

	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup
	for i, label := range labels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p.set(i, label+": fetching repositories...")
			onPage := func(n int) {
				p.set(i, fmt.Sprintf("%s: fetching repositories... %d", label, n))
			}
			var all []repo
			var err error
			if i == 0 {
				all, err = c.listMyRepos(ctx, onPage)
				personalErr = err
			} else {
				all, err = c.listOrgRepos(ctx, label, onPage)
			}
			if err != nil {
				p.settle(i, paint(os.Stdout, redBright, "✖"), fmt.Sprintf("%s: failed: %v", label, err))
				return
			}

			active := all
			if i > 0 {
				active = nil
				for _, r := range all {
					if !r.archived {
						active = append(active, r)
					}
				}
			}
			arch := ""
			if n := len(all) - len(active); n > 0 {
				arch = fmt.Sprintf(" (skipping %d archived)", n)
			}
			found[i] = active
			p.settle(i, paint(os.Stdout, greenBright, "✔"), fmt.Sprintf("%s: %d to clone%s", label, len(active), arch))
		}()
	}
	wg.Wait()

	if personalErr != nil {
		return nil, personalErr
	}
	var repos []repo
	for _, f := range found {
		repos = append(repos, f...)
	}
	return repos, nil
}

// confirm asks a yes/no question, defaulting to yes on an empty answer.
func confirm(ctx context.Context, message string) bool {
	fmt.Print(paint(os.Stdout, greenBright, "? ") + message + " (Y/n) ")
	answer := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		answer <- line
	}()
	select {
	case <-ctx.Done():
		fmt.Println()
		return false
	case line := <-answer:
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "y", "yes":
			return true
		}
		return false
	}
}

// Run executes the tool with the given command line (including the program
// name) and returns the process exit code.
func Run(args []string) int {
	opts, code, ok := parseArgs(args)
	if !ok {
		return code
	}
	if opts.config != "" && !exists(opts.config) {
		logError("The given config file does not exist: %s", opts.config)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiKey := gitConfig(opts.config, "github.apikey")
	if apiKey == "" {
		logError("No API Key found!")
		return 1
	}
	c := newClient(apiKey)

	base, err := basePath(opts)
	if err != nil {
		logError("%v", err)
		return 1
	}

	info("Listing orgs...")
	orgs, err := c.listOrgs(ctx)
	if err != nil {
		logError("Failed to list orgs: %v", err)
		return 1
	}

	info("Fetching repositories...")
	repos, err := fetchRepos(ctx, c, orgs)
	if err != nil {
		logError("Failed to fetch user repositories: %v", err)
		return 1
	}
	if len(repos) == 0 {
		info("You have no repos!")
		return 1
	}

	if !confirm(ctx, fmt.Sprintf("Are you sure you want to continue and clone %d repositories into %s:", len(repos), base)) {
		info("Aborted!")
		return 0
	}

	s := processRepos(ctx, base, repos, runtime.NumCPU()) // TODO: Add param?
	if ctx.Err() != nil {
		info("Aborted!")
		return 130
	}
	info("Done! %d cloned, %d skipped (already present), %d failed", s.cloned, s.skipped, len(s.failures))
	if len(s.failures) > 0 {
		for _, failure := range s.failures {
			logError("%s", failure)
		}
		return 1
	}
	return 0
}
