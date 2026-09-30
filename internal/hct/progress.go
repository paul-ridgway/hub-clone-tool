package hct

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// progress renders a block of live status lines, one per slot, each with a
// spinner until it is settled or finished. The most recently finished lines
// are kept above the slots. Without a TTY it only prints the settled and
// finished lines.
type slot struct {
	text   string
	mark   string // shown instead of the spinner once settled
	active bool
}

type progress struct {
	mu    sync.Mutex
	tty   bool
	slots []slot
	// recent holds the last maxRecent finished lines.
	recent []string
	drawn  int
	frame  int
	stop   chan struct{}
}

const maxRecent = 10

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func newProgress(workers int) *progress {
	p := &progress{tty: isTTY(os.Stdout), slots: make([]slot, workers), stop: make(chan struct{})}
	if p.tty {
		fmt.Print("\x1b[?7l\x1b[?25l") // no line wrap, so each status stays on one line; hide cursor
		go p.spin()
	}
	return p
}

func (p *progress) spin() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			p.frame = (p.frame + 1) % len(spinnerFrames)
			if p.drawn > 0 {
				p.redraw()
			}
			p.mu.Unlock()
		}
	}
}

func (p *progress) close() {
	close(p.stop)
	if p.tty {
		p.mu.Lock()
		defer p.mu.Unlock()
		fmt.Print("\x1b[?7h\x1b[?25h")
	}
}

func (p *progress) set(i int, line string) {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slots[i] = slot{text: line, active: true}
	p.redraw()
}

// settle stops the slot's spinner, leaving the line in place with the given mark.
func (p *progress) settle(i int, mark, line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.tty {
		fmt.Println(mark + " " + line)
		return
	}
	p.slots[i] = slot{text: line, mark: mark, active: true}
	p.redraw()
}

// finish frees the slot and adds line to the recently finished list.
func (p *progress) finish(i int, line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slots[i] = slot{}
	if !p.tty {
		fmt.Println(line)
		return
	}
	p.recent = append(p.recent, line)
	if len(p.recent) > maxRecent {
		p.recent = p.recent[1:]
	}
	p.redraw()
}

func (p *progress) redraw() {
	var b strings.Builder
	if p.drawn > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", p.drawn)
	}
	b.WriteString("\r\x1b[J")
	p.drawn = len(p.recent)
	for _, line := range p.recent {
		b.WriteString(line + "\n")
	}
	for _, s := range p.slots {
		if s.active {
			mark := s.mark
			if mark == "" {
				mark = paint(os.Stdout, yellowBright, spinnerFrames[p.frame])
			}
			b.WriteString(mark + " " + s.text + "\n")
			p.drawn++
		}
	}
	fmt.Print(b.String())
}
