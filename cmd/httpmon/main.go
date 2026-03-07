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

	"github.com/kostyay/httpmon/internal/bodydecoder"
	"github.com/kostyay/httpmon/internal/breakpoint"
	"github.com/kostyay/httpmon/internal/certutil"
	"github.com/kostyay/httpmon/internal/config"
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

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	flag.Int("port", 8080, "proxy listen port")
	dataDir := flag.String("data-dir", defaultDataDir(), "data directory for CA certs")
	flag.Int("buffer-size", 10000, "max flows in memory")
	blockHosts := flag.String("block", "", "comma-separated host patterns to block (wildcards: *.ads.com)")
	allowHosts := flag.String("allow", "", "comma-separated host patterns to allow (only these intercepted)")
	showVersion := flag.Bool("version", false, "print version and exit")
	installCA := flag.Bool("install-ca", false, "install CA cert into system trust store and exit")
	flag.String("throttle", "", "throttle preset: 3g, 4g, wifi")
	latencyFlag := flag.Duration("latency", 0, "added latency per response (e.g. 100ms)")
	flag.Bool("mcp", false, "start MCP server on default addr (127.0.0.1:9551)")
	flag.String("mcp-addr", "", "MCP server listen address (implies --mcp)")
	mcpTokenFlag := flag.Bool("mcp-token", false, "print MCP bearer token and exit")
	var protoPaths stringSlice
	flag.Var(&protoPaths, "proto-path", "path to .proto file or directory (repeatable)")
	var protoIncludes stringSlice
	flag.Var(&protoIncludes, "proto-include", "additional import search dir, like protoc -I (repeatable)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, cfgErr := config.Load(*dataDir)
	if cfgErr != nil {
		fatal("config: %v", cfgErr)
	}
	config.ApplyFlags(cfg, flag.Visit)

	// Merge proto paths: config first, CLI appended.
	if len(protoPaths) > 0 {
		cfg.ProtoPaths = append(cfg.ProtoPaths, protoPaths...)
	}
	if len(protoIncludes) > 0 {
		cfg.ProtoIncludes = append(cfg.ProtoIncludes, protoIncludes...)
	}

	// --mcp-addr implies --mcp.
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "mcp-addr" {
			cfg.MCPEnabled = true
		}
	})

	if *mcpTokenFlag {
		if err := config.LoadOrCreateToken(cfg, *dataDir); err != nil {
			fatal("mcp token: %v", err)
		}
		fmt.Println(cfg.MCPToken)
		return
	}

	if cfg.ProxyPort < 1 || cfg.ProxyPort > 65535 {
		fatal("invalid port: %d", cfg.ProxyPort)
	}
	if cfg.BufferSize < 1 {
		fatal("buffer-size must be > 0")
	}

	s := store.New(cfg.BufferSize)
	p := proxy.New(s, *dataDir)
	p.Resolver = procinfo.New(s)

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

	if cfg.ThrottlePreset != "" {
		bps := throttle.PresetBandwidth(cfg.ThrottlePreset)
		if bps == 0 {
			fatal("unknown throttle preset: %q (use 3g, 4g, or wifi)", cfg.ThrottlePreset)
		}
		p.ThrottleBPS = bps
	}
	if *latencyFlag > 0 {
		p.ThrottleLatency = *latencyFlag
	}

	// Build body decoder registry for protobuf/gRPC decoding.
	// Must be set before Init() so the interceptor can use it.
	protoDec := &bodydecoder.RawProtobufDecoder{}

	// Global (fallback) proto registry.
	var globalProtoReg *bodydecoder.ProtoRegistry
	if len(cfg.ProtoPaths) > 0 {
		reg, protoErrs := bodydecoder.LoadProtoFiles(cfg.ProtoPaths, cfg.ProtoIncludes...)
		for _, e := range protoErrs {
			fmt.Fprintf(os.Stderr, "proto: %v\n", e)
		}
		globalProtoReg = reg
	}

	// Per-host proto registries.
	var hostRegs map[string]*bodydecoder.ProtoRegistry
	if len(cfg.ProtoHosts) > 0 {
		hostRegs = make(map[string]*bodydecoder.ProtoRegistry, len(cfg.ProtoHosts))
		for pattern, hcfg := range cfg.ProtoHosts {
			reg, protoErrs := bodydecoder.LoadProtoFiles(hcfg.Paths, hcfg.Includes...)
			for _, e := range protoErrs {
				fmt.Fprintf(os.Stderr, "proto[%s]: %v\n", pattern, e)
			}
			hostRegs[pattern] = reg
		}
	}

	if globalProtoReg != nil || len(hostRegs) > 0 {
		har := bodydecoder.NewHostAwareRegistry(globalProtoReg, hostRegs)
		protoDec.ResolveReg = har.Resolver()
	}
	grpcDec := &bodydecoder.GRPCWebDecoder{Proto: protoDec}
	decoderReg := bodydecoder.NewRegistry(grpcDec, protoDec)
	p.DecoderRegistry = decoderReg

	addr := fmt.Sprintf(":%d", cfg.ProxyPort)
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

	var mcpSrv *mcpserver.Server
	if cfg.MCPEnabled {
		if err := config.LoadOrCreateToken(cfg, *dataDir); err != nil {
			fatal("mcp token: %v", err)
		}
		mcpSrv = mcpserver.New(mcpserver.Config{
			Store:    s,
			Proxy:    p,
			Scripts:  mgr,
			Throttle: p,
			Addr:     cfg.MCPAddr,
			Token:    cfg.MCPToken,
		})
		if err := mcpSrv.Start(ctx); err != nil {
			fatal("mcp server: %v", err)
		}
		fmt.Fprintf(os.Stderr, "MCP server listening on %s (token: %s)\n", mcpSrv.Addr(), cfg.MCPToken)
	}

	tuiCfg := tui.AppConfig{
		Store:       s,
		Proxy:       p,
		CATrusted:   caTrusted,
		Scripts:     mgr,
		Throttle:    p,
		Breakpoints: bpCtrl,
		DataDir:     *dataDir,
		BodyDecoder: decoderReg,
	}
	if mcpSrv != nil {
		tuiCfg.MCP = mcpSrv
	}
	app := tui.NewApp(tuiCfg)
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
