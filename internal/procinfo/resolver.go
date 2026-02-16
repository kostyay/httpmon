package procinfo

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	psnet "github.com/shirou/gopsutil/v3/net"
	psproc "github.com/shirou/gopsutil/v3/process"

	"github.com/kostyay/httpmon/internal/store"
)

const (
	maxConcurrent = 10
	failThreshold = 5
	fallbackName  = "\u2014" // em dash
)

// flowStore is the subset of store.RingBuffer the resolver needs.
type flowStore interface {
	Update(id store.FlowID, fn func(*store.FlowMeta))
	UpdateData(id store.FlowID, fn func(*store.FlowData))
}

// Resolver resolves which OS process owns a proxied connection.
type Resolver struct {
	store    flowStore
	sem      chan struct{}
	pidCache sync.Map // int32 → string

	failCount atomic.Int32
	warnOnce  sync.Once
	wg        sync.WaitGroup

	// injectable for testing
	findPID        func(port uint16) (int32, error)
	processName    func(pid int32) (string, error)
	processCmdline func(pid int32) (string, error)
}

// New creates a Resolver backed by the given store.
func New(s *store.RingBuffer) *Resolver {
	r := &Resolver{
		store: s,
		sem:   make(chan struct{}, maxConcurrent),
	}
	r.findPID = r.defaultFindPID
	r.processName = r.defaultProcessName
	r.processCmdline = r.defaultProcessCmdline
	return r
}

// Resolve spawns a background goroutine to resolve the process for flowID.
func (r *Resolver) Resolve(flowID string, clientPort uint16) {
	r.wg.Add(1)
	r.sem <- struct{}{} // acquire semaphore
	go func() {
		defer r.wg.Done()
		defer func() { <-r.sem }() // release semaphore
		r.resolve(flowID, clientPort)
	}()
}

// Wait blocks until all pending resolutions complete. Used in tests.
func (r *Resolver) Wait() {
	r.wg.Wait()
}

func (r *Resolver) resolve(flowID string, clientPort uint16) {
	pid, err := r.findPID(clientPort)
	if err != nil {
		r.store.Update(flowID, func(m *store.FlowMeta) {
			m.Process = fallbackName
		})
		r.recordFailure()
		return
	}

	name, err := r.cachedProcessName(pid)
	if err != nil {
		name = fallbackName
		r.recordFailure()
	} else {
		r.failCount.Store(0)
	}

	cmdline, _ := r.processCmdline(pid)

	r.store.Update(flowID, func(m *store.FlowMeta) {
		m.Process = name
		m.ProcessPID = pid
	})
	r.store.UpdateData(flowID, func(d *store.FlowData) {
		d.ProcessPID = pid
		d.ProcessCmdline = cmdline
	})
}

func (r *Resolver) cachedProcessName(pid int32) (string, error) {
	if name, ok := r.pidCache.Load(pid); ok {
		return name.(string), nil
	}
	name, err := r.processName(pid)
	if err != nil {
		return "", err
	}
	r.pidCache.Store(pid, name)
	return name, nil
}

func (r *Resolver) recordFailure() {
	count := r.failCount.Add(1)
	if count >= failThreshold {
		r.warnOnce.Do(func() {
			log.Printf(
				"warning: process resolution failed %d consecutive times; "+
					"consider running with sudo or granting permissions",
				count,
			)
		})
	}
}

func (r *Resolver) defaultFindPID(port uint16) (int32, error) {
	conns, err := psnet.Connections("tcp")
	if err != nil {
		return 0, err
	}
	for _, c := range conns {
		if c.Laddr.Port == uint32(port) {
			return c.Pid, nil
		}
	}
	return 0, fmt.Errorf("no connection found for port %d", port)
}

func (r *Resolver) defaultProcessName(pid int32) (string, error) {
	p, err := psproc.NewProcess(pid)
	if err != nil {
		return "", err
	}
	return p.Name()
}

func (r *Resolver) defaultProcessCmdline(pid int32) (string, error) {
	p, err := psproc.NewProcess(pid)
	if err != nil {
		return "", err
	}
	return p.Cmdline()
}
