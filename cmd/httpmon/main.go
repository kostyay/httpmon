package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/certutil"
	"github.com/kostyay/httpmon/internal/hostfilter"
	"github.com/kostyay/httpmon/internal/mcpserver"
	"github.com/kostyay/httpmon/internal/procinfo"
	"github.com/kostyay/httpmon/internal/proxy"
	"github.com/kostyay/httpmon/internal/scripting"
	"github.com/kostyay/httpmon/internal/store"
	"github.com/kostyay/httpmon/internal/throttle"
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
	throttleFlag := flag.String("throttle", "", "throttle preset: 3g, 4g, wifi")
	latencyFlag := flag.Duration("latency", 0, "added latency per response (e.g. 100ms)")
	mcpFlag := flag.Bool("mcp", false, "start MCP server on default addr (127.0.0.1:9551)")
	mcpAddrFlag := flag.String("mcp-addr", "", "MCP server listen address (implies --mcp)")
	mcpTokenFlag := flag.Bool("mcp-token", false, "print MCP bearer token and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *mcpTokenFlag {
		token, err := mcpserver.LoadOrCreateToken(*dataDir)
		if err != nil {
			fatal("mcp token: %v", err)
		}
		fmt.Println(token)
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
	p.Resolver = procinfo.New(s)

	// Init scripting engine and breakpoint controller.
	scriptsDir := filepath.Join(*dataDir, "scripts")
	engine := scripting.New()
	engine.LoadFromDir(scriptsDir)
	bpCtrl := breakpoint.NewController()
	p.ScriptEngine = engine
	p.BreakpointCtrl = bpCtrl

	if *blockHosts != "" || *allowHosts != "" {
		block := splitCSV(*blockHosts)
		allow := splitCSV(*allowHosts)
		p.HostFilter = hostfilter.New(block, allow)
	}

	// Throttle configuration.
	if *throttleFlag != "" {
		bps := throttle.PresetBandwidth(*throttleFlag)
		if bps == 0 {
			fatal("unknown throttle preset: %q (use 3g, 4g, or wifi)", *throttleFlag)
		}
		p.ThrottleBPS = bps
	}
	if *latencyFlag > 0 {
		p.ThrottleLatency = *latencyFlag
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

	// MCP server (optional).
	if *mcpAddrFlag != "" {
		*mcpFlag = true
	}
	var mcpSrv *mcpserver.Server
	if *mcpFlag {
		mcpAddr := *mcpAddrFlag
		if mcpAddr == "" {
			mcpAddr = mcpserver.DefaultAddr
		}
		token, tokenErr := mcpserver.LoadOrCreateToken(*dataDir)
		if tokenErr != nil {
			fatal("mcp token: %v", tokenErr)
		}
		mcpSrv = mcpserver.New(mcpserver.Config{
			Store:    s,
			Proxy:    p,
			Scripts:  mgr,
			Throttle: p,
			Addr:     mcpAddr,
			Token:    token,
		})
		if err := mcpSrv.Start(ctx); err != nil {
			fatal("mcp server: %v", err)
		}
		fmt.Fprintf(os.Stderr, "MCP server listening on %s (token: %s)\n", mcpSrv.Addr(), token)
	}

	cfg := tui.AppConfig{
		Store:       s,
		Proxy:       p,
		CATrusted:   caTrusted,
		Scripts:     mgr,
		Throttle:    p,
		Breakpoints: bpCtrl,
		MCP:         mcpSrv,
	}
	app := tui.NewApp(cfg)
	prog := tea.NewProgram(app)
	if _, err := prog.Run(); err != nil {
		fatal("TUI error: %v", err)
	}
	cancel()
	if mcpSrv != nil {
		mcpSrv.Stop()
	}
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
