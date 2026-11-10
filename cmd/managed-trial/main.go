//go:build managedtrial

// Command managed-trial is the managed Go trial host for Hyperclay Local
// documents. It is a trial program, not part of the release: every file of it
// carries the managedtrial build tag.
//
//	managed-trial --root DIR [--root DIR ...] --port N --local-source DIR [--person-name NAME] [--control-fd FD]
//
// It prints exactly one READY line on stdout, logs to stderr, and exits 0 on
// SIGTERM, SIGINT or stdin EOF.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/panphora/htmlclay/internal/managedtrial"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	if err := managedtrial.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "managed-trial: %v\n", err)
		os.Exit(1)
	}
}
