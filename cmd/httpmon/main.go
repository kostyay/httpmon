package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/certutil"
	"github.com/kostyay/httpmon/internal/proxy"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/tui"
)

var version = "dev"

func main() {
	port := flag.Int("port", 8080, "proxy listen port")
	dataDir := flag.String("data-dir", defaultDataDir(), "data directory for CA certs")
	bufSize := flag.Int("buffer-size", 10000, "max flows in memory")
	showVersion := flag.Bool("version", false, "print version and exit")
	installCA := flag.Bool("install-ca", false, "install CA cert into system trust store and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *port < 1 || *port > 65535 {
		fatal("invalid port: %d", *port)
	}
	if *bufSize < 1 {
		fatal("buffer-size must be > 0")
	}

	s := store.New(*bufSize)
	p := proxy.New(s, *dataDir)

	addr := fmt.Sprintf(":%d", *port)
	if err := p.Init(addr); err != nil {
		fatal("proxy init: %v", err)
	}

	if *installCA {
		if err := certutil.Install(p.CACertPath()); err != nil {
			fatal("install CA: %v", err)
		}
		fmt.Println("CA certificate installed successfully:", p.CACertPath())
		return
	}

	fmt.Fprintf(os.Stderr, "CA cert: %s\n", p.CACertPath())
	fmt.Fprintf(os.Stderr, "Proxy listening on %s\n", addr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	proxyErr := make(chan error, 1)
	go func() { proxyErr <- p.Serve(ctx) }()

	caTrusted := certutil.IsInstalled(p.CACertPath())
	app := tui.NewApp(s, p, caTrusted)
	prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := prog.Run(); err != nil {
		fatal("TUI error: %v", err)
	}
	cancel()
	p.Stop()
	if err := <-proxyErr; err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".httpmon"
	}
	return filepath.Join(home, ".httpmon")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "httpmon: "+format+"\n", args...)
	os.Exit(1)
}
