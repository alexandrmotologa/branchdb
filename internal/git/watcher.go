package git

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ChangeHandler is called whenever the active Git branch shifts.
type ChangeHandler func(oldBranch, newBranch string)

// Watcher observes Git repository changes and provides thread-safe branch queries.
type Watcher struct {
	resolver      *Resolver
	mu            sync.RWMutex
	currentBranch string
	override      string
	handlers      []ChangeHandler
	stopCh        chan struct{}
}

// NewWatcher creates a new Git repository watcher.
func NewWatcher(resolver *Resolver, initialOverride string) *Watcher {
	w := &Watcher{
		resolver: resolver,
		override: initialOverride,
		stopCh:   make(chan struct{}),
	}

	if initialOverride != "" {
		w.currentBranch = initialOverride
	} else {
		branch, _ := resolver.GetActiveBranch()
		if branch == "" {
			branch = "main"
		}
		w.currentBranch = branch
	}

	return w
}

// OnChange registers a branch switch listener.
func (w *Watcher) OnChange(h ChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, h)
}

// CurrentBranch returns the currently active branch or manual override.
func (w *Watcher) CurrentBranch() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.override != "" {
		return w.override
	}
	return w.currentBranch
}

// SetOverride temporarily forces a specific branch regardless of Git HEAD.
func (w *Watcher) SetOverride(branch string) {
	w.mu.Lock()
	old := w.currentBranch
	if w.override != "" {
		old = w.override
	}
	w.override = branch
	w.mu.Unlock()

	w.notify(old, branch)
}

// ClearOverride removes manual branch override and resumes following Git HEAD.
func (w *Watcher) ClearOverride() {
	w.mu.Lock()
	old := w.override
	w.override = ""
	actual, _ := w.resolver.GetActiveBranch()
	w.currentBranch = actual
	w.mu.Unlock()

	w.notify(old, actual)
}

func (w *Watcher) notify(oldBranch, newBranch string) {
	if oldBranch == newBranch || newBranch == "" {
		return
	}
	w.mu.RLock()
	handlers := append([]ChangeHandler(nil), w.handlers...)
	w.mu.RUnlock()

	for _, h := range handlers {
		go h(oldBranch, newBranch)
	}
}

// Start begins file system notification and fallback polling.
func (w *Watcher) Start(ctx context.Context) error {
	gitDir, err := w.resolver.FindGitDir()
	var fsWatcher *fsnotify.Watcher
	if err == nil {
		fsWatcher, err = fsnotify.NewWatcher()
		if err == nil {
			defer fsWatcher.Close()
			_ = fsWatcher.Add(gitDir)
			headPath := filepath.Join(gitDir, "HEAD")
			_ = fsWatcher.Add(headPath)
		}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.stopCh:
			return nil
		case event, ok := <-func() <-chan fsnotify.Event {
			if fsWatcher != nil {
				return fsWatcher.Events
			}
			return nil
		}():
			if !ok {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				w.checkBranch()
			}
		case err, ok := <-func() <-chan error {
			if fsWatcher != nil {
				return fsWatcher.Errors
			}
			return nil
		}():
			if ok && err != nil {
				log.Printf("[git-watcher] fsnotify error: %v", err)
			}
		case <-ticker.C:
			w.checkBranch()
		}
	}
}

func (w *Watcher) checkBranch() {
	w.mu.RLock()
	hasOverride := w.override != ""
	prev := w.currentBranch
	w.mu.RUnlock()

	if hasOverride {
		return
	}

	active, err := w.resolver.GetActiveBranch()
	if err != nil || active == "" {
		return
	}

	if active != prev {
		w.mu.Lock()
		w.currentBranch = active
		w.mu.Unlock()
		log.Printf("[git-watcher] Active branch changed: %s -> %s", prev, active)
		w.notify(prev, active)
	}
}

// Stop terminates the watcher loop.
func (w *Watcher) Stop() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
}
