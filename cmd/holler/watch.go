package main

import (
	"context"
	"os"

	"charm.land/log/v2"

	"github.com/hollerprotocol/holler/internal/tui"
)

func init() {
	// main.go's init (which fills the table) runs first: files initialize
	// in name order.
	commands = append(commands, command{"top", "", "alias for watch", cmdWatch})
}

func cmdWatch(ctx context.Context, args []string) error {
	f := newFlags("watch", "", "A live dashboard of the agents on the holler network: who is connected,\nwhat each is working on, every thread's state and the activity as it happens.\nIt reads this host's daemon (it does not start one). Agents on other hosts\nappear when they share presence (holler up --presence).")
	debug := f.String("debug-log", "", "write a debug log to this file")
	if err := f.Parse(args); err != nil {
		return err
	}
	var opts tui.Options
	if *debug != "" {
		lf, err := os.OpenFile(*debug, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		defer lf.Close()
		opts.Log = log.NewWithOptions(lf, log.Options{ReportTimestamp: true, Level: log.DebugLevel, Prefix: "watch"})
	}
	return tui.Run(ctx, tui.ControlSource{C: f.client()}, opts)
}
