// Package diag renders the receiver's live diagnostic display.
package diag

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/session"
)

// Lines renders a snapshot as three lines of text.
func Lines(s session.Snapshot) [3]string {
	var l [3]string
	if !s.Connected {
		l[0] = "○ waiting for a controller…"
	} else {
		bat := "?"
		if s.Battery <= 100 {
			bat = fmt.Sprintf("%d%%", s.Battery)
			if s.Charging {
				bat += "+"
			}
		}
		rtt := "–"
		if s.PhoneRTT > 0 {
			rtt = ms(s.PhoneRTT)
		}
		l[0] = fmt.Sprintf("● %s  %s  %3.0f Hz  rtt %s  jitter %s  battery %s",
			s.Device, s.Addr, s.RateHz, rtt, ms(s.Jitter), bat)
	}

	st := s.State
	buttons := strings.Join(st.Buttons.Names(), " ")
	if s.Released {
		buttons = "all released"
	}
	l[1] = fmt.Sprintf("  L %s %s  R %s %s  LT %s  RT %s  D-pad %-2s  [%s]",
		axis(st.LeftX), axis(st.LeftY), axis(st.RightX), axis(st.RightY),
		bar(st.LeftTrigger), bar(st.RightTrigger), st.DPad, buttons)

	rumble := ""
	if s.Rumble != [2]uint16{} {
		rumble = fmt.Sprintf("  rumble %d%%/%d%%", int(s.Rumble[0])*100/0xFFFF, int(s.Rumble[1])*100/0xFFFF)
	}
	l[2] = fmt.Sprintf("  packets %d ok, %d lost, %d stale, %d forged, %d malformed, %d blocked  proc p50 %s p99 %s  watchdog %d%s",
		s.Accepted, s.Lost, s.Stale, s.AuthFailed, s.Malformed, s.Blocked,
		us(s.ProcP50), us(s.ProcP99), s.WatchdogReleases, rumble)
	return l
}

func axis(v float32) string { return fmt.Sprintf("%+.2f", v) }

func bar(v float32) string {
	const w = 5
	n := int(v*w + 0.5)
	return "[" + strings.Repeat("█", n) + strings.Repeat("·", w-n) + "]"
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond)) }

func us(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return ms(d)
}

// Display redraws the lines in place on an ANSI terminal.
type Display struct {
	w     io.Writer
	drawn bool
}

// NewDisplay returns a Display writing to w.
func NewDisplay(w io.Writer) *Display {
	enableVT()
	return &Display{w: w}
}

// Draw replaces the previous frame.
func (d *Display) Draw(s session.Snapshot) {
	var b strings.Builder
	if d.drawn {
		b.WriteString("\x1b[3F") // up three lines, column 0
	}
	for _, line := range Lines(s) {
		b.WriteString("\x1b[2K")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	io.WriteString(d.w, b.String())
	d.drawn = true
}

// Plain prints one line per call, for logs and non-terminal output.
func Plain(w io.Writer, s session.Snapshot) {
	l := Lines(s)
	fmt.Fprintf(w, "%s |%s |%s\n", l[0], l[1], l[2])
}
