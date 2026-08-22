package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Andyccr/RainIRC/internal/config"
	"github.com/Andyccr/RainIRC/internal/identity"
	"github.com/Andyccr/RainIRC/internal/logger"
	"github.com/Andyccr/RainIRC/internal/node"
	"github.com/Andyccr/RainIRC/internal/ui"
	"github.com/Andyccr/RainIRC/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "p2pirc: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}
	if cfg.ShowVersion {
		fmt.Println(version.String())
		return nil
	}
	log := logger.NewDefault(cfg.Debug)
	ident, err := identity.LoadOrCreate(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("identity: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	n, err := node.Start(ctx, cfg, ident, log)
	if err != nil {
		return err
	}
	defer n.Close()

	return ui.New(n).Run(ctx)
}
