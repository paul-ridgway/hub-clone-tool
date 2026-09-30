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
// spinner until it is settled or finished, repainted on each spinner tick. The most recently finished lines
// are kept beneath the slots, and an optional footer anchored to the bottom of the terminal. The block is fitted to the terminal height, as it
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
	// footer, if set, supplies a status line anchored to the bottom row of
	// the terminal, outside the scrolling region.
	footer func() string
	// anchored is the terminal height the footer's row was reserved for.
	anchored int
	closed   bool
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
			if p.closed {
				p.mu.Unlock()
				return
			}
			p.frame = (p.frame + 1) % len(spinnerFrames)
			p.redraw()
			p.mu.Unlock()
		}
	}
}

func (p *progress) close() {
	close(p.stop)
	if p.tty {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.closed = true
		p.redraw()
		if p.anchored > 0 {
			// Clear the footer and give its row back to the scrolling region.
			fmt.Printf("\x1b7\x1b[r\x1b[%d;1H\x1b[2K\x1b8", p.anchored)
			p.anchored = 0
		}
		fmt.Print("\x1b[?7h\x1b[?25h")
	}
}

// anchor reserves the bottom row of the terminal for the footer by shrinking
// the scrolling region, redoing it if the terminal has been resized.
func (p *progress) anchor(b *strings.Builder) int {
	h := p.height()
	if h == p.anchored {
		return h
	}
	if p.anchored == 0 {
		// Make room first, in case the cursor is already on the bottom row.
		fmt.Fprintf(b, "\n\x1b7\x1b[1;%dr\x1b8\x1b[1A", h-1)
	} else {
		fmt.Fprintf(b, "\x1b7\x1b[1;%dr\x1b8", h-1)
	}
	p.anchored = h
	return h
}

// set shows a spinner line for the slot, with an optional detail beneath it.
func (p *progress) set(i int, line, detail string) {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slots[i] = slot{text: line, detail: detail, active: true}
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

// layout returns the lines to draw: the active slots, then as many recent
// lines as fit beneath them. Details move onto the slot's own line if the terminal is too
// short for two lines each, and slots that still don't fit are summarised.
func (p *progress) layout() []string {
	rows := p.height() - 1 // the cursor sits on the line below the block
	if p.footer != nil {
		rows--
	}
	rows = max(rows, 2)

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

	// Finished lines flow down from beneath the active slots, newest first.
	for i := len(p.recent) - 1; i >= 0 && len(lines) < rows; i-- {
		lines = append(lines, p.recent[i])
	}
	return lines
}

// redraw repaints the block in place. It is called on each spinner tick, so
// updates between ticks are batched, and lines are overwritten rather than
// cleared first to avoid flicker.
func (p *progress) redraw() {
	var b strings.Builder
	b.WriteString("\x1b[?2026h") // synchronised update, where supported
	bottom := 0
	if p.footer != nil {
		bottom = p.anchor(&b)
	}
	if p.drawn > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", p.drawn)
	}
	b.WriteString("\r")
	lines := p.layout()
	for _, line := range lines {
		b.WriteString(line + "\x1b[K\n")
	}
	if stale := p.drawn - len(lines); stale > 0 {
		b.WriteString(strings.Repeat("\x1b[K\n", stale))
		fmt.Fprintf(&b, "\x1b[%dA", stale)
	}
	p.drawn = len(lines)
	if p.footer != nil {
		fmt.Fprintf(&b, "\x1b7\x1b[%d;1H%s\x1b[K\x1b8", bottom, p.footer())
	}
	b.WriteString("\x1b[?2026l")
	fmt.Print(b.String())
}
