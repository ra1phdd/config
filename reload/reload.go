// Package reload provides optional configuration reload support.
package reload

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/ra1phdd/config/v2"
)

// ErrNotStarted indicates that Reload was called before Start.
var ErrNotStarted = errors.New("config reloader not started")

// ErrClosed indicates that the reloader has completed its lifecycle.
var ErrClosed = errors.New("config reloader is closed")

// Update is a reload result. Err leaves the last valid application snapshot unchanged.
type Update[T any] struct {
	Config T
	Err    error
}

type reloaderOptions struct {
	files    []string
	debounce time.Duration
	trigger  Trigger
}

// Trigger starts a change-detection strategy. Its channel sends nil for a
// change and a non-nil error when detection fails, and closes when ctx ends.
type Trigger interface {
	Start(context.Context) (<-chan error, error)
}

// Option configures a reloader.
type Option func(*reloaderOptions)

// WithWatchFile adds a file whose changes trigger a full reload.
func WithWatchFile(path string) Option {
	return func(o *reloaderOptions) { o.files = append(o.files, path) }
}

// WithDebounce coalesces bursts of file changes.
func WithDebounce(delay time.Duration) Option {
	return func(o *reloaderOptions) {
		if delay >= 0 {
			o.debounce = delay
		}
	}
}

// WithTrigger selects an independent change-detection strategy.
func WithTrigger(trigger Trigger) Option {
	return func(o *reloaderOptions) { o.trigger = trigger }
}

// Reloader publishes complete load attempts after a trigger signal.
type Reloader[T any] struct {
	loader     *config.Loader[T]
	trigger    Trigger
	lifecycle  sync.Mutex
	mu         sync.Mutex
	loadMu     sync.Mutex
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	started    bool
	closed     bool
	current    T
	hasCurrent bool
}

// New creates a reloader for an immutable loader and a change trigger.
func New[T any](loader *config.Loader[T], opts ...Option) (*Reloader[T], error) {
	if loader == nil {
		return nil, errors.New("nil config loader")
	}
	o := reloaderOptions{debounce: 100 * time.Millisecond}
	for _, option := range opts {
		if option != nil {
			option(&o)
		}
	}
	if o.trigger != nil && len(o.files) != 0 {
		return nil, errors.New("configure either a trigger or watched files")
	}
	if o.trigger == nil && len(o.files) == 0 {
		return nil, errors.New("at least one watched file is required")
	}
	if o.trigger == nil && slices.Contains(o.files, "") {
		return nil, errors.New("watched file path must not be empty")
	}
	trigger := o.trigger
	if trigger == nil {
		trigger = fileTrigger{files: append([]string(nil), o.files...), debounce: o.debounce}
	}
	return &Reloader[T]{loader: loader, trigger: trigger}, nil
}

// Start loads the initial snapshot and starts change detection.
func (r *Reloader[T]) Start(ctx context.Context) (<-chan Update[T], error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("starting reloader: %w", err)
	}
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrClosed
	}
	if r.started {
		r.mu.Unlock()
		return nil, errors.New("reloader already started")
	}
	r.started = true
	r.mu.Unlock()
	watchCtx, cancel := context.WithCancel(ctx)
	events, stateErr := r.trigger.Start(watchCtx)
	if stateErr != nil {
		cancel()
		r.mu.Lock()
		r.started = false
		r.mu.Unlock()
		return nil, fmt.Errorf("start reload trigger: %w", stateErr)
	}
	initial, err := r.loader.Load(ctx)
	if err != nil {
		cancel()
		r.mu.Lock()
		r.started = false
		r.mu.Unlock()
		return nil, fmt.Errorf("load initial configuration: %w", err)
	}
	r.mu.Lock()
	r.cancel = cancel
	initial = r.loader.Clone(initial)
	r.current = initial
	r.hasCurrent = true
	r.mu.Unlock()
	updates := make(chan Update[T], 1)
	updates <- Update[T]{Config: r.loader.Clone(initial)}
	r.wg.Add(1)
	go r.watch(watchCtx, updates, events)
	return updates, nil
}

// Current returns the last successfully loaded snapshot.
func (r *Reloader[T]) Current() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loader.Clone(r.current), r.hasCurrent
}

// Reload loads a new snapshot without waiting for a watched file change.
// On failure, the last successful snapshot remains available through Current.
// Reload requires Start to have initialized the reloader state.
func (r *Reloader[T]) Reload(ctx context.Context) (Update[T], error) {
	if err := ctx.Err(); err != nil {
		wrapped := fmt.Errorf("reloading configuration: %w", err)
		return Update[T]{Err: wrapped}, wrapped
	}
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return Update[T]{Err: ErrClosed}, ErrClosed
	}
	started := r.started && r.hasCurrent
	r.mu.Unlock()
	if !started {
		err := ErrNotStarted
		return Update[T]{Err: err}, err
	}
	cfg, err := r.loadSnapshot(ctx)
	if err != nil {
		return Update[T]{Err: err}, err
	}
	return Update[T]{Config: r.loader.Clone(cfg)}, nil
}

func (r *Reloader[T]) loadSnapshot(ctx context.Context) (T, error) {
	r.loadMu.Lock()
	defer r.loadMu.Unlock()
	cfg, err := r.loader.Load(ctx)
	if err != nil {
		return cfg, fmt.Errorf("load configuration snapshot: %w", err)
	}
	cfg = r.loader.Clone(cfg)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return cfg, ErrClosed
	}
	r.current = cfg
	r.hasCurrent = true
	r.mu.Unlock()
	return cfg, nil
}

// Close stops the watcher and closes its update channel.
func (r *Reloader[T]) Close() error {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	return nil
}

func (r *Reloader[T]) watch(ctx context.Context, updates chan<- Update[T], events <-chan error) {
	defer r.wg.Done()
	defer close(updates)
	for {
		select {
		case <-ctx.Done():
			return
		case triggerErr, ok := <-events:
			if !ok {
				return
			}
			if triggerErr != nil {
				select {
				case updates <- Update[T]{Err: triggerErr}:
				case <-ctx.Done():
					return
				}
				continue
			}
			cfg, err := r.loadSnapshot(ctx)
			update := Update[T]{Config: r.loader.Clone(cfg), Err: err}
			select {
			case updates <- update:
			case <-ctx.Done():
				return
			}
		}
	}
}

type fileTrigger struct {
	files    []string
	debounce time.Duration
}

func (t fileTrigger) Start(ctx context.Context) (<-chan error, error) {
	state, err := fileState(t.files, nil)
	if err != nil {
		return nil, err
	}
	events := make(chan error, 1)
	go t.watch(ctx, events, state)
	return events, nil
}

func (t fileTrigger) watch(ctx context.Context, events chan<- error, state map[string]fileStamp) {
	defer close(events)
	interval := max(t.debounce/2, 10*time.Millisecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var changedAt time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := fileState(t.files, state)
			if err != nil {
				select {
				case events <- err:
				case <-ctx.Done():
					return
				}
				continue
			}
			if !sameState(state, current) {
				state = current
				changedAt = time.Now()
			}
			if changedAt.IsZero() || time.Since(changedAt) < t.debounce {
				continue
			}
			changedAt = time.Time{}
			select {
			case events <- nil:
			case <-ctx.Done():
				return
			}
		}
	}
}

type fileStamp struct {
	digest [32]byte
	exists bool
	size   int64
	mtime  int64
	mode   os.FileMode
}

func fileState(paths []string, previous map[string]fileStamp) (map[string]fileStamp, error) {
	state := make(map[string]fileStamp, len(paths))
	var readErr error
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				readErr = errors.Join(readErr, &config.FieldError{Source: path, Path: path, Op: "read", Err: err})
			}
			state[path] = fileStamp{}
			continue
		}
		stamp := fileStamp{exists: true, size: info.Size(), mtime: info.ModTime().UnixNano(), mode: info.Mode()}
		if before, ok := previous[path]; ok && sameMetadata(before, stamp) {
			stamp.digest = before.digest
			state[path] = stamp
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // Paths are explicit caller-configured watch targets.
		if err != nil {
			readErr = errors.Join(readErr, &config.FieldError{Source: path, Path: path, Op: "read", Err: err})
			state[path] = fileStamp{}
			continue
		}
		stamp.digest = sha256.Sum256(data)
		state[path] = stamp
	}
	return state, readErr
}

func sameMetadata(left, right fileStamp) bool {
	return left.exists == right.exists && left.exists && left.size == right.size && left.mtime == right.mtime && left.mode == right.mode
}

func sameState(left, right map[string]fileStamp) bool {
	if len(left) != len(right) {
		return false
	}
	for path, before := range left {
		after, ok := right[path]
		if !ok || before != after {
			return false
		}
	}
	return true
}
