package main

import (
	"fmt"
	"io"
	"time"

	"github.com/hollerprotocol/holler/internal/api"
	"github.com/hollerprotocol/holler/internal/render"
)

// printer writes events for people, printing a thread banner whenever the
// thread changes.
type printer struct {
	w          io.Writer
	showThread bool
	lastTh     string
}

func (p *printer) event(ev api.Event) {
	if p.showThread && ev.Th != "" && ev.Th != p.lastTh {
		subject := ""
		if ev.Subject != "" {
			subject = fmt.Sprintf(" %q", ev.Subject)
		}
		fmt.Fprintf(p.w, "── %s%s with %s\n", ev.Th, subject, ev.PeerName)
		p.lastTh = ev.Th
	}
	fmt.Fprint(p.w, render.Event(ev, render.Options{Clock: true}))
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Local().Format("Jan 2")
}

func humanSize(n int64) string { return render.HumanSize(n) }
