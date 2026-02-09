# Phase 1 — Core Pipeline & Basic TUI

## Goal

Proxy intercepts HTTP/HTTPS traffic, stores flows in memory, TUI shows scrollable list + detail view with substring filter. Ship working software you can actually use.

## Architecture

```
cmd/httpmon/main.go       Wiring, CLI flags, lifecycle
internal/proxy/           go-mitmproxy wrapper + interceptor addon + CA cert
internal/store/           Thread-safe ring buffer
internal/filter/          Substring filter function
internal/tui/             Bubble Tea: list view + detail view
```

No `internal/engine/` package. No interfaces until a second implementation exists. Types live in the packages that use them. `pkg/` not used (single binary, not a library).

## Design Decisions

- **Concrete types, no premature interfaces** — TUI imports `*store.RingBuffer` directly. Extract interfaces when gRPC adapter arrives (30-min refactor).
- **Single `sync.RWMutex`** on the store, not per-flow mutexes. One writer (proxy), one reader (TUI). Profile later.
- **TUI polls on tick** — 100ms tick calls `store.List()`. No subscriber channels, no fan-out, no event coalescing. Bubble Tea already batches renders.
- **Bodies stored with details** — `map[FlowID]*FlowData` holds headers + bodies. Ring buffer `[]FlowMeta` for the list. One mutex guards both. Bodies evicted when FlowMeta overwritten.
- **Two views, not a screen stack** — `showingDetail bool` + `selectedFlowID`. Add stack when 3+ screens exist.
- **Use `http.Header` directly** — don't reinvent it.
- **`q` quits only when no text input focused.**

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Port in use / CA cert failure | Exit with stderr error before TUI starts |
| Everything else | Log + continue |

Proxy `Init()` runs synchronously (CA gen + port bind). Only the accept loop runs async. No race between init and TUI.

---

## Step 1 — Engine: Types + Store + Proxy + Filter

Build the data pipeline: proxy captures traffic → store holds it → filter narrows it.

### 1a. Types (`internal/store/types.go`)

```go
type FlowID = string // use go-mitmproxy's UUID directly
type FlowState int   // StateInProgress, StateCompleted, StateFailed

type FlowMeta struct {
    ID          FlowID
    Method      string
    StatusCode  int
    Host        string
    Path        string
    Duration    time.Duration
    SizeBytes   int64
    StartedAt   time.Time
    State       FlowState
    ContentType string
    Scheme      string
}

type FlowData struct {
    RequestHeaders  http.Header
    RequestBody     []byte
    ResponseHeaders http.Header
    ResponseBody    []byte
}

// Filter is the only interface needed — it's consumed by store.List()
type Filter interface {
    Match(flow *FlowMeta) bool
}
```

No `FlowDetail` composite — `Get()` returns `(*FlowMeta, *FlowData, error)` as separate values. No `QueryParams` (parse from Path on demand). No `Trailers`, `TLSVersion`, `Protocol`, `RemoteAddr` — add when needed. No `BodyTruncated` — check `len(body) == maxBodySize` at display time.

### 1b. Ring Buffer (`internal/store/ringbuffer.go`)

```go
type RingBuffer struct {
    mu       sync.RWMutex
    metas    []FlowMeta          // fixed-capacity circular buffer
    data     map[FlowID]*FlowData // headers + bodies
    index    map[FlowID]int      // FlowID → ring index for O(1) lookup
    head     int
    count    int
    capacity int
}

func New(capacity int) *RingBuffer
func (rb *RingBuffer) Add(meta FlowMeta)
func (rb *RingBuffer) Update(id FlowID, fn func(*FlowMeta))
func (rb *RingBuffer) SetData(id FlowID, data *FlowData)
func (rb *RingBuffer) Get(id FlowID) (*FlowMeta, *FlowData, error)
func (rb *RingBuffer) List(filter Filter, offset, limit int) ([]FlowMeta, int)
func (rb *RingBuffer) Len() int
```

- `Add()` evicts oldest slot + its data entry + index entry on overflow
- `List()` iterates newest-first; second return is total count matching filter
- `SetData()` replaces (not merges) the FlowData for an ID — interceptor builds full FlowData progressively then sets it
- No `Subscribe`, `Count`, `Purge` — add when needed

**Tests:**
- [ ] Add/Get round-trip
- [ ] Eviction at capacity+1: oldest gone, data cleaned, index cleaned
- [ ] Update() with concurrent goroutines + `-race`
- [ ] List() with nil filter (all), with filter, with offset/limit
- [ ] Get() for evicted ID returns error

### 1c. Quick Filter (`internal/filter/quick.go`)

Phase 1 filter: **case-insensitive substring on host+path**. That's it.

```go
type QuickFilter struct {
    query string // lowercased
}

func CompileQuick(input string) *QuickFilter

func (f *QuickFilter) Match(flow *FlowMeta) bool {
    target := strings.ToLower(flow.Host + flow.Path)
    return strings.Contains(target, f.query)
}
```

No method prefix, status prefix, negation — add when users ask.

**Tests:**
- [ ] Empty input matches all
- [ ] Case insensitive
- [ ] Matches host, path, combined

### 1d. MITM Proxy (`internal/proxy/`)

**proxy.go:**
```go
type Proxy struct {
    mp      *mitmproxy.Proxy
    store   *store.RingBuffer
    dataDir string
    addr    string
    caCert  string
}

func New(store *store.RingBuffer, dataDir string) *Proxy
func (p *Proxy) Init(addr string) error   // sync: CA gen + port bind
func (p *Proxy) Serve(ctx context.Context) // async: accept loop
func (p *Proxy) Stop()
func (p *Proxy) Addr() string
func (p *Proxy) CACertPath() string
```

Split startup: `Init()` runs synchronously (CA gen, validates port). `Serve()` runs in goroutine.

**interceptor.go** — go-mitmproxy addon:
- `Requestheaders` → create FlowMeta{State: InProgress}, store.Add()
- `Request` → capture request headers + body (max 5MB), store.SetData() with request part
- `Responseheaders` → store.Update() with status code
- `Response` → capture response headers + body (decoded via `DecodedBody()`), update FlowMeta with duration/size/State=Completed, store.SetData() with full data
- All hooks: `defer recover()` → log + mark flow Failed

**cert.go:**
- Check `~/.httpmon/ca.pem` + `~/.httpmon/ca-key.pem`
- If absent: generate 2048-bit RSA CA, write with `0600` perms
- Return path for user trust instruction

**Tests:**
- [ ] Start proxy, HTTP request through it, flow in store with correct fields
- [ ] HTTPS with generated CA
- [ ] 100 concurrent requests with `-race` — no data races
- [ ] Body >5MB stored at exactly 5MB
- [ ] `Init()` with port 0 (random) works; used port returns error

---

## Step 2 — TUI: List + Detail

### 2a. App Model (`internal/tui/app.go`)

```go
type App struct {
    store   *store.RingBuffer
    proxy   *proxy.Proxy

    // view state
    showDetail  bool
    selectedID  store.FlowID
    selectedIdx int

    // flow list state
    flows       []store.FlowMeta
    filterInput textinput.Model
    filterText  string
    filter      store.Filter // nil = match all

    width, height int
    ready         bool
    statusMsg     string // transient message in status bar
}
```

- `Init()` returns `tea.Tick(100ms)` for polling
- On tick: call `store.List(filter, 0, height-3)` to rebuild flow slice
- `q` quits only when `!filterInput.Focused()`
- `ctrl+c` always quits

### 2b. Flow List View

**Layout:**
```
/ to filter...                              [quick]
─────────────────────────────────────────────────
METHOD STATUS HOST              PATH           DUR    SIZE
GET    200    api.stripe.com    /v1/charges    45ms   1.2K
POST   201    api.stripe.com    /v1/tokens     123ms  456B
─────────────────────────────────────────────────
42 flows | Proxy :8080                    ? help  / filter
```

- Columns: Method(7), Status(6), Host(40%), Path(remaining), Duration(7), Size(7)
- Method column width 7 (fits "DELETE" + pad)
- Color-code methods + status codes inline (5-6 lipgloss styles, no Theme struct)
- `j`/`k`/arrows navigate, `g`/`G` top/bottom
- `/` focuses filter input, `Esc` blurs
- `Enter` opens detail view
- Empty state: "Waiting for traffic... proxy at :8080"

### 2c. Detail View

**Layout:**
```
< Esc    GET 200 https://api.stripe.com/v1/charges    45ms  1.2K
─────────────────────────────────────────────────────────────────
[Request]  Response

▸ General
  Method: GET
  URL: https://api.stripe.com/v1/charges
  Scheme: https

▸ Headers (8)
  Accept: application/json
  Authorization: Bearer sk_test_...
  ...

▸ Body
  {"amount": 2000, "currency": "usd"}
```

- **Two tabs**: Request + Response (no Timing — data unavailable; no Export — Phase 2)
- Left/Right or `1`/`2` switch tabs
- `j`/`k` scroll viewport
- `Esc` returns to list
- `n`/`N` next/prev flow without going back to list
- InProgress flow: Response tab shows "Awaiting response..."
- Evicted flow: show "Flow no longer available", return to list on any key
- Body shown inline, truncated to 50 lines. No separate body viewer screen for now.

### 2d. Styles (inline, no Theme struct)

Define 5-6 package-level `lipgloss.Style` vars:
- `styleStatus2xx`, `styleStatus4xx`, `styleStatus5xx`
- `styleMethodColor` (bold)
- `styleSelected` (inverted bg)
- `styleMuted` (gray)
- `styleStatusBar` (bg highlight)

Use `lipgloss.AdaptiveColor` for light/dark support.

**Tests:**
- [ ] App.Update with KeyMsg 'j' increments selectedIdx
- [ ] App.Update with KeyMsg 'enter' sets showDetail=true
- [ ] App.Update with KeyMsg 'esc' in detail sets showDetail=false
- [ ] App.View in list mode contains "flows"
- [ ] App.View in detail mode contains header info
- [ ] Filter input focus/blur cycle
- [ ] `q` when filter focused does not quit

---

## Step 3 — Wiring + Ship

### `cmd/httpmon/main.go`

```go
func main() {
    port := flag.Int("port", 8080, "proxy listen port")
    dataDir := flag.String("data-dir", defaultDataDir(), "data directory")
    bufSize := flag.Int("buffer-size", 10000, "max flows in memory")
    verbose := flag.Bool("verbose", false, "verbose logging")
    version := flag.Bool("version", false, "print version")
    flag.Parse()

    if *version { fmt.Println(ver); return }

    // Validate
    if *port < 1 || *port > 65535 { fatal("invalid port") }
    if *bufSize < 1 { fatal("buffer-size must be >0") }

    s := store.New(*bufSize)
    p := proxy.New(s, *dataDir)

    addr := fmt.Sprintf(":%d", *port)
    if err := p.Init(addr); err != nil { fatal(err) }

    fmt.Fprintf(os.Stderr, "CA cert: %s\n", p.CACertPath())
    fmt.Fprintf(os.Stderr, "Proxy listening on %s\n", addr)

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()
    go p.Serve(ctx)

    app := tui.NewApp(s, p)
    prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
    prog.Run()
    p.Stop()
}
```

**Acceptance:**
- [ ] `httpmon --version` prints version
- [ ] `httpmon -p 9090` starts on 9090
- [ ] Invalid port → error + exit
- [ ] Ctrl+C → clean shutdown
- [ ] CA path + proxy addr printed to stderr before TUI

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/lqqyt2423/go-mitmproxy` | MITM proxy |
| `github.com/charmbracelet/bubbletea` | TUI framework |
| `github.com/charmbracelet/lipgloss` | Styling |
| `github.com/charmbracelet/bubbles` | textinput, viewport |

Removed from Phase 1: `chroma` (no syntax highlighting in inline body preview), `uuid` (reuse go-mitmproxy's Flow.Id).

## Deferred to Phase 1.5 / Phase 2

- cURL export + clipboard
- Body viewer screen (full-screen, syntax highlighted, search)
- Help screen
- Timing tab (need httptrace integration)
- Export tab
- Screen stack abstraction
- Toast/notification system
- Subscriber/fan-out channels
- Theme struct
- Filter: method prefix, status prefix, negation
- Benchmarks
- teatest integration tests
- Cross-platform clipboard (OSC 52 + xclip)

## Unresolved Questions

1. **go-mitmproxy hook ordering**: Do `Requestheaders`→`Request`→`Responseheaders`→`Response` fire sequentially per flow, or can hooks for same flow overlap? Determines if interceptor needs its own locking.

2. **go-mitmproxy `DecodedBody()`**: Does this exist? Or do we need to decompress gzip/br manually? Affects whether `SizeBytes` reflects wire or decoded size.

3. **go-mitmproxy CONNECT**: Does it handle HTTPS CONNECT tunneling transparently, or do we need explicit support?
