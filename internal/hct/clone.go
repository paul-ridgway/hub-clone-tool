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
	"syscall"
	"time"
)

type stats struct {
	cloned, skipped int
	// failures holds one "org / name: reason" line per failed clone.
	failures []string
}

// tally tracks overall progress across the clone workers.
type tally struct {
	mu      sync.Mutex
	total   int
	started time.Time
	stats
}

func (t *tally) update(f func(*stats)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f(&t.stats)
}

const barWidth = 30

// statusBar renders the overall progress line anchored to the bottom of the terminal.
func (t *tally) statusBar() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	done := t.cloned + t.skipped + len(t.failures)
	filled := 0
	if t.total > 0 {
		filled = barWidth * done / t.total
	}
	bar := paint(os.Stdout, greenBright, strings.Repeat("█", filled)) + paint(os.Stdout, dim, strings.Repeat("░", barWidth-filled))
	line := fmt.Sprintf("%s %d/%d (%d%%) · %d cloned · %d skipped", bar, done, t.total, 100*done/max(t.total, 1), t.cloned, t.skipped)
	if n := len(t.failures); n > 0 {
		line += " · " + paint(os.Stdout, redBright, fmt.Sprintf("%d failed", n))
	}
	return line + " · " + time.Since(t.started).Round(time.Second).String()
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
	// Interrupt rather than kill, so git removes its partial clone.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
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

func cloneRepo(ctx context.Context, p *progress, t *tally, slot int, base string, r repo) {
	label := r.org + " / " + r.name
	path := pathForRepo(base, r)

	if _, err := os.Stat(path); err == nil {
		t.update(func(s *stats) { s.skipped++ })
		p.finish(slot, paint(os.Stdout, yellowBright, "↓ ")+label+" [skipped: target exists]")
		return
	}

	p.set(slot, label, "Starting...")
	err := runClone(ctx, r, path, func(line string) {
		p.set(slot, label, line)
	})
	if ctx.Err() != nil {
		p.finish(slot, paint(os.Stdout, dim, "– "+label+" [aborted]"))
		return
	}
	if err != nil {
		failure := label + ": " + err.Error()
		t.update(func(s *stats) { s.failures = append(s.failures, failure) })
		p.finish(slot, paint(os.Stdout, redBright, "✖ ")+failure)
		return
	}
	t.update(func(s *stats) { s.cloned++ })
	p.finish(slot, paint(os.Stdout, greenBright, "✔ ")+label)
}

func processRepos(ctx context.Context, base string, repos []repo, workers int) stats {
	t := &tally{total: len(repos), started: time.Now()}
	p := newProgress(workers)
	p.footer = t.statusBar
	defer p.close()

	queue := make(chan repo)
	var wg sync.WaitGroup
	for slot := 0; slot < workers; slot++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range queue {
				cloneRepo(ctx, p, t, slot, base, r)
			}
		}()
	}
	for _, r := range repos {
		select {
		case queue <- r:
			continue
		case <-ctx.Done():
		}
		break
	}
	close(queue)
	wg.Wait()
	return t.stats
}
