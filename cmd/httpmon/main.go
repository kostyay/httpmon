package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"os/signal"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kostyay/httpmon/internal/certutil"
	"github.com/kostyay/httpmon/internal/hostfilter"
	"github.com/kostyay/httpmon/internal/proxy"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/tui"
)

var version = "dev"

func main() {
	port := flag.Int("port", 8080, "proxy listen port")
	dataDir := flag.String("data-dir", defaultDataDir(), "data directory for CA certs")
	bufSize := flag.Int("buffer-size", 10000, "max flows in memory")
	blockHosts := flag.String("block", "", "comma-separated host patterns to block (wildcards: *.ads.com)")
	allowHosts := flag.String("allow", "", "comma-separated host patterns to allow (only these intercepted)")
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

	// Init scripting engine.
	scriptsDir := filepath.Join(*dataDir, "scripts")
	engine := scripting.New()
	engine.LoadFromDir(scriptsDir)
	p.ScriptEngine = engine

	if *blockHosts != "" || *allowHosts != "" {
		block := splitCSV(*blockHosts)
		allow := splitCSV(*allowHosts)
		p.HostFilter = hostfilter.New(block, allow)
	}

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
	mgr := scripting.NewManager(engine, scriptsDir)
	app := tui.NewApp(s, p, caTrusted, mgr)
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "httpmon: "+format+"\n", args...)
	os.Exit(1)
}
