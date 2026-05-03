package ui

import (
	"context"
	"os"
	pathpkg "path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// garminFITWatchTriggerMsg is sent when new or updated .fit files appear in Garmin FIT dir.
type garminFITWatchTriggerMsg struct {
	Paths []string // cleaned absolute paths
}

type fitWatcherCtl struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

func newFitWatcherCtl() *fitWatcherCtl {
	return &fitWatcherCtl{}
}

func (c *fitWatcherCtl) Restart(dir string, outgoing chan<- tea.Msg) {
	dir = strings.TrimSpace(dir)

	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()

	if outgoing == nil {
		return
	}

	dir = pathpkg.Clean(dir)
	if dir == "" || dir == "." {
		return
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	go runGarminFITWatcher(ctx, dir, outgoing)
}

func runGarminFITWatcher(ctx context.Context, dir string, outgoing chan<- tea.Msg) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			_ = err
		}
	}()

	if err := watcher.Add(dir); err != nil {
		return
	}

	pathIn := make(chan string, 256)
	go debounceFITIntoTea(ctx, pathIn, outgoing, 650*time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-watcher.Errors:
			_ = err
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			ext := strings.ToLower(pathpkg.Ext(ev.Name))
			if ext != ".fit" {
				continue
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Chmod) == 0 {
				continue
			}
			select {
			case pathIn <- ev.Name:
			case <-ctx.Done():
				return
			default:
				// drop if burst exceeds buffer; next event will retry
				select {
				case pathIn <- ev.Name:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}
}

func debounceFITIntoTea(ctx context.Context, in <-chan string, out chan<- tea.Msg, wait time.Duration) {
	for {
		var path string
		select {
		case <-ctx.Done():
			return
		case p := <-in:
			path = pathpkg.Clean(p)
		}
		if path == "" {
			continue
		}

		pending := map[string]struct{}{path: {}}

	outerDrain:
		for {
			select {
			case p := <-in:
				cp := pathpkg.Clean(p)
				if cp != "" {
					pending[cp] = struct{}{}
				}
			default:
				break outerDrain
			}
		}

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}

		for {
			select {
			case p := <-in:
				cp := pathpkg.Clean(p)
				if cp != "" {
					pending[cp] = struct{}{}
				}
			default:
				goto emit
			}
		}
	emit:
		if len(pending) == 0 {
			continue
		}
		paths := make([]string, 0, len(pending))
		for p := range pending {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		select {
		case out <- garminFITWatchTriggerMsg{Paths: paths}:
		case <-ctx.Done():
			return
		default:
			// never block UI; watcher will fire again after import
		}
	}
}
