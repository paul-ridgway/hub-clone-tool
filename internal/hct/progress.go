package hct

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// progress renders a block of live status lines, one per slot, each with a
// spinner until it is settled or finished. The most recently finished lines
// are kept above the slots. The block is fitted to the terminal height, as it
// can only be redrawn in place while it is all on screen. Without a TTY it
// only prints the settled and finished lines.
type slot struct {
	text   string
	detail string // optional status, shown on a second line when there is room
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
	// rows reports the terminal height; nil means query the terminal.
	rows func() int
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

// set shows a spinner line for the slot, with an optional detail beneath it.
func (p *progress) set(i int, line, detail string) {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slots[i] = slot{text: line, detail: detail, active: true}
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

func (p *progress) height() int {
	if p.rows != nil {
		return p.rows()
	}
	if _, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && h > 0 {
		return h
	}
	return 24
}

// layout returns the lines to draw: as many recent lines as fit above the
// active slots. Details move onto the slot's own line if the terminal is too
// short for two lines each, and slots that still don't fit are summarised.
func (p *progress) layout() []string {
	rows := max(p.height()-1, 2) // the cursor sits on the line below the block

	var active []slot
	needed := 0
	for _, s := range p.slots {
		if s.active {
			active = append(active, s)
			needed++
			if s.detail != "" {
				needed++
			}
		}
	}
	twoLine := needed <= rows

	var lines []string
	for _, s := range active {
		mark := s.mark
		if mark == "" {
			mark = paint(os.Stdout, yellowBright, spinnerFrames[p.frame])
		}
		line := mark + " " + s.text
		switch {
		case s.detail == "":
			lines = append(lines, line)
		case twoLine:
			lines = append(lines, line, "  "+paint(os.Stdout, dim, "→ "+s.detail))
		default:
			lines = append(lines, line+" "+paint(os.Stdout, dim, s.detail))
		}
	}
	if len(lines) > rows {
		hidden := len(lines) - (rows - 1)
		lines = append(lines[:rows-1], paint(os.Stdout, dim, fmt.Sprintf("  … and %d more", hidden)))
	}

	recent := p.recent[len(p.recent)-min(len(p.recent), rows-len(lines)):]
	return append(append([]string{}, recent...), lines...)
}

func (p *progress) redraw() {
	var b strings.Builder
	if p.drawn > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", p.drawn)
	}
	b.WriteString("\r\x1b[J")
	lines := p.layout()
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	p.drawn = len(lines)
	fmt.Print(b.String())
}
