package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type fileMapBindOptions struct {
	configPath    string
	directoryRef  string
	format        string
	allowSymlinks bool
	strictJSON    bool
	resolver      PathResolver
}

type fileMapBindingTarget interface {
	bindFileMap(fileMapBindOptions) error
	syncFileMap() error
	fileMapBound() bool
}

type directoryFileMapStore[T any] struct {
	directory     string
	extension     string
	allowSymlinks bool
	strictJSON    bool
}

func (m *FileMap[T]) bindFileMap(options fileMapBindOptions) error {
	if err := validateFileMapElementType(reflect.TypeFor[T](), make(map[reflect.Type]bool)); err != nil {
		return err
	}
	ref := strings.TrimSpace(m.ref)
	if ref == "" {
		ref = strings.TrimSpace(options.directoryRef)
	}
	if ref == "" {
		return fmt.Errorf("%w: directory reference is empty", ErrInvalidFileMapEntry)
	}
	if !strings.HasPrefix(ref, FileScheme) {
		ref = FileScheme + ref
	}
	directory, err := resolveFileReferencePath(options.configPath, ref, options.resolver)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFileMapEntry, err)
	}
	format, err := normalizeFileMapFormat(options.format)
	if err != nil {
		return err
	}
	store := &directoryFileMapStore[T]{directory: directory, extension: "." + format, allowSymlinks: options.allowSymlinks, strictJSON: options.strictJSON}
	items, err := store.load()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.store = store
	m.items = items
	m.ref = ref
	m.mu.Unlock()
	return nil
}

func (m *FileMap[T]) fileMapBound() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store != nil
}

func (m *FileMap[T]) syncFileMap() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.store == nil {
		return ErrFileMapUnbound
	}
	for key, value := range m.items {
		if err := m.store.write(key, value, nil); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFileMapFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "", "json":
		return "json", nil
	case "yaml":
		return "yaml", nil
	case "yml":
		return "yml", nil
	default:
		return "", fmt.Errorf("%w: unsupported format %q", ErrInvalidFileMapEntry, format)
	}
}

func (s *directoryFileMapStore[T]) load() (map[string]T, error) {
	out := make(map[string]T)
	info, statErr := os.Lstat(s.directory)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 && !s.allowSymlinks {
			return nil, fmt.Errorf("%w: file map directory is a symlink", ErrUnsafePath)
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("%w: file map path is not a directory", ErrUnsafePath)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("%w: inspect directory: %w", ErrInvalidFileMapEntry, statErr)
	}
	entries, err := os.ReadDir(s.directory)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read directory: %w", ErrInvalidFileMapEntry, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), s.extension) {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if err := validateFileMapKey(key); err != nil {
			return nil, fmt.Errorf("%w: entry %q: %w", ErrInvalidFileMapEntry, entry.Name(), err)
		}
		path := filepath.Join(s.directory, entry.Name())
		data, err := readRegularFile(path, s.allowSymlinks)
		if err != nil {
			return nil, fmt.Errorf("%w: read key %q: %w", ErrInvalidFileMapEntry, key, err)
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, fmt.Errorf("%w: key %q is empty", ErrInvalidFileMapEntry, key)
		}
		var value T
		if err := decodeConfigFileInto(path, data, &value, s.strictJSON); err != nil {
			return nil, fmt.Errorf("%w: decode key %q: %w", ErrInvalidFileMapEntry, key, err)
		}
		out[key] = value
	}
	return out, nil
}

func (s *directoryFileMapStore[T]) write(key string, value T, mustExist *bool) error {
	path, err := s.entryPath(key)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(path)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("write key %q: %w", key, statErr)
	}
	if mustExist != nil && exists != *mustExist {
		return fmt.Errorf("%w: key %q", ErrFileMapConflict, key)
	}
	data, err := encodeConfigFile(path, value)
	if err != nil {
		return fmt.Errorf("%w: encode key %q: %w", ErrInvalidFileMapEntry, key, err)
	}
	if err := writePrivateFile(path, data, s.allowSymlinks); err != nil {
		return fmt.Errorf("write key %q: %w", key, err)
	}
	return nil
}
func (s *directoryFileMapStore[T]) remove(key string) error {
	path, err := s.entryPath(key)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("%w: key %q", ErrFileMapConflict, key)
	}
	if err != nil {
		return fmt.Errorf("delete key %q: %w", key, err)
	}
	if info.Mode()&os.ModeSymlink != 0 && !s.allowSymlinks {
		return fmt.Errorf("%w: key %q is a symlink", ErrUnsafePath, key)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%w: key %q is not a regular file", ErrUnsafePath, key)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete key %q: %w", key, err)
	}
	if dir, err := os.Open(s.directory); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *directoryFileMapStore[T]) entryPath(key string) (string, error) {
	if err := validateFileMapKey(key); err != nil {
		return "", err
	}
	return filepath.Join(s.directory, key+s.extension), nil
}

func validateFileMapKey(key string) error {
	if key == "" || key == "." || key == ".." || strings.TrimSpace(key) != key ||
		strings.ContainsAny(key, `/\:`) || filepath.IsAbs(key) || filepath.Base(key) != key {
		return fmt.Errorf("%w: %q", ErrInvalidFileMapKey, key)
	}
	return nil
}

func bindDirectoryFileMaps(target any, configPath string, allowSymlinks bool, strictJSON bool, resolver PathResolver) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return ErrNilConfig
	}
	return walkDirectoryFileMaps(v.Elem(), configPath, allowSymlinks, strictJSON, resolver, false)
}

func bindUnboundDirectoryFileMaps(target any, configPath string, allowSymlinks bool, strictJSON bool, resolver PathResolver) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return ErrNilConfig
	}
	return walkDirectoryFileMaps(v.Elem(), configPath, allowSymlinks, strictJSON, resolver, true)
}

func syncDirectoryFileMaps(target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ErrNilConfig
		}
		v = v.Elem()
	}
	return walkSyncDirectoryFileMaps(v)
}

func walkSyncDirectoryFileMaps(v reflect.Value) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fieldType, fieldValue := t.Field(i), v.Field(i)
		if !fieldType.IsExported() {
			continue
		}
		_, _, tagged := parseFileMapConfigTag(fieldType.Tag.Get("config"))
		if tagged {
			if !fieldValue.CanAddr() {
				continue
			}
			target, ok := fieldValue.Addr().Interface().(fileMapBindingTarget)
			if !ok {
				return fmt.Errorf("%w: field %s with dir tag must be FileMap", ErrInvalidFileMapEntry, fieldType.Name)
			}
			if err := target.syncFileMap(); err != nil {
				return fmt.Errorf("sync file map %s: %w", fieldType.Name, err)
			}
			continue
		}
		if indirectType(fieldType.Type).Kind() == reflect.Struct {
			if err := walkSyncDirectoryFileMaps(fieldValue); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkDirectoryFileMaps(v reflect.Value, configPath string, allowSymlinks bool, strictJSON bool, resolver PathResolver, onlyUnbound bool) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fieldType, fieldValue := t.Field(i), v.Field(i)
		if !fieldType.IsExported() {
			continue
		}
		directory, format, ok := parseFileMapConfigTag(fieldType.Tag.Get("config"))
		if ok {
			if !fieldValue.CanAddr() {
				continue
			}
			target, ok := fieldValue.Addr().Interface().(fileMapBindingTarget)
			if !ok {
				return fmt.Errorf("%w: field %s with dir tag must be FileMap", ErrInvalidFileMapEntry, fieldType.Name)
			}
			if onlyUnbound && target.fileMapBound() {
				continue
			}
			if err := target.bindFileMap(fileMapBindOptions{configPath: configPath, directoryRef: directory, format: format, allowSymlinks: allowSymlinks, strictJSON: strictJSON, resolver: resolver}); err != nil {
				return fmt.Errorf("bind file map %s: %w", fieldType.Name, err)
			}
			continue
		}
		kind := indirectType(fieldType.Type).Kind()
		if kind == reflect.Struct {
			if err := walkDirectoryFileMaps(fieldValue, configPath, allowSymlinks, strictJSON, resolver, onlyUnbound); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFileMapElementType(t reflect.Type, seen map[reflect.Type]bool) error {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if isSecureType(t) || typeContainsSecurity(t, make(map[reflect.Type]bool)) {
		return fmt.Errorf("%w: type %s contains protected values", ErrInvalidFileMapEntry, t)
	}
	if seen[t] {
		return nil
	}
	seen[t] = true
	fileMapPackage := reflect.TypeFor[FileMap[struct{}]]().PkgPath()
	if t.PkgPath() == fileMapPackage && strings.HasPrefix(t.Name(), "FileMap[") {
		return fmt.Errorf("%w: nested FileMap type %s", ErrInvalidFileMapEntry, t)
	}
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if err := validateFileMapElementType(t.Field(i).Type, seen); err != nil {
				return err
			}
		}
	case reflect.Map:
		if err := validateFileMapElementType(t.Key(), seen); err != nil {
			return err
		}
		return validateFileMapElementType(t.Elem(), seen)
	case reflect.Slice, reflect.Array:
		return validateFileMapElementType(t.Elem(), seen)
	}
	return nil
}

func parseFileMapConfigTag(tag string) (directory string, format string, ok bool) {
	for _, raw := range strings.Split(tag, ",") {
		part := strings.TrimSpace(raw)
		if strings.HasPrefix(part, "dir=") {
			directory, ok = strings.TrimSpace(strings.TrimPrefix(part, "dir=")), true
		}
		if strings.HasPrefix(part, "format=") {
			format = strings.TrimSpace(strings.TrimPrefix(part, "format="))
		}
	}
	return directory, format, ok
}
