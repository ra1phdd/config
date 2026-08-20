package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// FileMap is a thread-safe typed map backed by one config file per key.
// Do not copy a FileMap after first use.
type FileMap[T any] struct {
	mu    sync.RWMutex
	items map[string]T
	store fileMapStore[T]
	ref   string
}

type fileMapStore[T any] interface {
	write(key string, value T, mustExist *bool) error
	remove(key string) error
	load() (map[string]T, error)
}

// NewFileMap returns an empty, unbound FileMap.
func NewFileMap[T any]() FileMap[T] { return FileMap[T]{items: make(map[string]T)} }

// Get returns the value stored under key.
func (m *FileMap[T]) Get(key string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.items[key]
	return value, ok
}

// All returns a shallow copy of the entries map.
func (m *FileMap[T]) All() map[string]T {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]T, len(m.items))
	for key, value := range m.items {
		out[key] = value
	}
	return out
}

// Keys returns all logical keys in lexical order.
func (m *FileMap[T]) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.items))
	for key := range m.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Len returns the number of loaded entries.
func (m *FileMap[T]) Len() int { m.mu.RLock(); defer m.mu.RUnlock(); return len(m.items) }

// Add persists a value under a key that must not already exist.
func (m *FileMap[T]) Add(key string, value T) error {
	exists := false
	return m.write(key, value, &exists)
}

// Update persists a value under a key that must already exist.
func (m *FileMap[T]) Update(key string, value T) error {
	exists := true
	return m.write(key, value, &exists)
}

// Set persists a value, inserting or replacing its key.
func (m *FileMap[T]) Set(key string, value T) error { return m.write(key, value, nil) }

func (m *FileMap[T]) write(key string, value T, mustExist *bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return ErrFileMapUnbound
	}
	_, exists := m.items[key]
	if mustExist != nil && exists != *mustExist {
		return fmt.Errorf("%w: key %q", ErrFileMapConflict, key)
	}
	if err := m.store.write(key, value, mustExist); err != nil {
		return err
	}
	if m.items == nil {
		m.items = make(map[string]T)
	}
	m.items[key] = value
	return nil
}

// Delete removes an existing key from disk and memory.
func (m *FileMap[T]) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return ErrFileMapUnbound
	}
	if _, exists := m.items[key]; !exists {
		return fmt.Errorf("%w: key %q", ErrFileMapConflict, key)
	}
	if err := m.store.remove(key); err != nil {
		return err
	}
	delete(m.items, key)
	return nil
}

// Reload atomically replaces memory with the complete directory contents.
func (m *FileMap[T]) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return ErrFileMapUnbound
	}
	items, err := m.store.load()
	if err != nil {
		return err
	}
	m.items = items
	return nil
}

func (m *FileMap[T]) UnmarshalJSON(data []byte) error {
	var ref string
	if err := json.Unmarshal(data, &ref); err != nil {
		return err
	}
	m.ref = ref
	return nil
}

func (m *FileMap[T]) MarshalJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.Marshal(m.ref)
}

func (m *FileMap[T]) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("file map reference must be a scalar")
	}
	m.ref = node.Value
	return nil
}

func (m *FileMap[T]) MarshalYAML() (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ref, nil
}
