package hct

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type result int

const (
	cloned result = iota
	skipped
	failed
)

type stats struct {
	cloned, skipped int
	// failures holds one "org / name: reason" line per failed clone.
	failures []string
}

// scanProgress splits git's output on both \n and \r, as progress updates are
// rewritten in place with carriage returns.
func scanProgress(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func runClone(ctx context.Context, r repo, path string, update func(string)) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--progress", r.gitURL, path)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	last := ""
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanProgress)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			last = line
			update(line)
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if last != "" {
			return errors.New(last)
		}
		return err
	}
	return nil
}

func cloneRepo(ctx context.Context, p *progress, slot int, base string, r repo) (result, string) {
	label := r.org + " / " + r.name
	path := pathForRepo(base, r)

	if _, err := os.Stat(path); err == nil {
		p.finish(slot, paint(os.Stdout, yellowBright, "↓ ")+label+" [skipped: target exists]")
		return skipped, ""
	}

	title := fmt.Sprintf("Cloning from %s into %s...", label, path)
	p.set(slot, title)
	err := runClone(ctx, r, path, func(line string) {
		p.set(slot, title+" "+line)
	})
	if err != nil {
		failure := label + ": " + err.Error()
		p.finish(slot, paint(os.Stdout, redBright, "✖ ")+failure)
		return failed, failure
	}
	p.finish(slot, paint(os.Stdout, greenBright, "✔ ")+label)
	return cloned, ""
}

func processRepos(ctx context.Context, base string, repos []repo, workers int) stats {
	p := newProgress(workers)
	defer p.close()

	queue := make(chan repo)
	type outcome struct {
		res     result
		failure string
	}
	results := make(chan outcome)

	var wg sync.WaitGroup
	for slot := 0; slot < workers; slot++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range queue {
				res, failure := cloneRepo(ctx, p, slot, base, r)
				results <- outcome{res, failure}
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, r := range repos {
			select {
			case queue <- r:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var s stats
	for o := range results {
		switch o.res {
		case cloned:
			s.cloned++
		case skipped:
			s.skipped++
		case failed:
			s.failures = append(s.failures, o.failure)
		}
	}
	return s
}
