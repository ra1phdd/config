package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

type fileMapUser struct {
	Name string `json:"name" yaml:"name"`
	Age  int    `json:"age" yaml:"age"`
}

func TestFileMapConcurrentReadsAndWrites(t *testing.T) {
	users, _ := loadBoundFileMapForTest(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("user-%d", i)
			if err := users.Set(key, fileMapUser{Name: key}); err != nil {
				t.Errorf("Set() error = %v", err)
			}
		}(i)
		go func() { defer wg.Done(); _ = users.All(); _ = users.Keys(); _ = users.Len() }()
	}
	wg.Wait()
	if users.Len() != 20 {
		t.Fatalf("Len() = %d, want 20", users.Len())
	}
}

func TestFileMapWriteFailureDoesNotChangeMemory(t *testing.T) {
	users, usersDir := loadBoundFileMapForTest(t)
	if err := os.WriteFile(usersDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := users.Set("one", fileMapUser{Name: "One"})
	if err == nil {
		t.Fatal("Set() error = nil, want filesystem error")
	}
	if users.Len() != 0 {
		t.Fatalf("failed Set changed memory: %#v", users.All())
	}
}

type fileMapApp struct {
	Name  string               `json:"name" yaml:"name"`
	Users FileMap[fileMapUser] `json:"users" yaml:"users" config:"dir=users,format=json"`
}

func TestFileMapLoadsDirectoryFromMainReference(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	usersDir := filepath.Join(dir, "users")
	if err := os.MkdirAll(usersDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"name":"app","users":"file://users/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usersDir, "123.json"), []byte(`{"name":"Alice","age":30}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usersDir, "456.json"), []byte(`{"name":"Bob","age":40}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadGeneric(func() *fileMapApp { return &fileMapApp{} }, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if got.Name != "app" || got.Users.Len() != 2 {
		t.Fatalf("config = %#v, len = %d", got, got.Users.Len())
	}
	if alice, ok := got.Users.Get("123"); !ok || alice.Name != "Alice" || alice.Age != 30 {
		t.Fatalf("Get(123) = %#v, %v", alice, ok)
	}
	if !reflect.DeepEqual(got.Users.Keys(), []string{"123", "456"}) {
		t.Fatalf("Keys() = %v", got.Users.Keys())
	}
}

func TestFileMapMalformedEntryFailsWholeLoad(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	usersDir := filepath.Join(dir, "users")
	if err := os.MkdirAll(usersDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"users":"file://users/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usersDir, "bad.json"), []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadGeneric(func() *fileMapApp { return &fileMapApp{} }, WithConfigPath(configPath), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if !errors.Is(err, ErrInvalidFileMapEntry) {
		t.Fatalf("error = %v, want ErrInvalidFileMapEntry", err)
	}
}

func TestFileMapCRUDPersistsIndividualFiles(t *testing.T) {
	users, usersDir := loadBoundFileMapForTest(t)
	if err := users.Add("123", fileMapUser{Name: "Alice", Age: 30}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	assertFileMapUserFile(t, filepath.Join(usersDir, "123.json"), fileMapUser{Name: "Alice", Age: 30})
	if err := users.Add("123", fileMapUser{}); !errors.Is(err, ErrFileMapConflict) {
		t.Fatalf("duplicate Add error = %v", err)
	}
	if err := users.Update("123", fileMapUser{Name: "Alice", Age: 31}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	assertFileMapUserFile(t, filepath.Join(usersDir, "123.json"), fileMapUser{Name: "Alice", Age: 31})
	if err := users.Update("missing", fileMapUser{}); !errors.Is(err, ErrFileMapConflict) {
		t.Fatalf("missing Update error = %v", err)
	}
	if err := users.Set("456", fileMapUser{Name: "Bob", Age: 40}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if users.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", users.Len())
	}
	if err := users.Delete("123"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(usersDir, "123.json")); !os.IsNotExist(err) {
		t.Fatalf("deleted file stat error = %v", err)
	}
	if err := users.Delete("123"); !errors.Is(err, ErrFileMapConflict) {
		t.Fatalf("missing Delete error = %v", err)
	}
}

func TestFileMapRejectsUnsafeKeys(t *testing.T) {
	users, _ := loadBoundFileMapForTest(t)
	for _, key := range []string{"", ".", "..", "../x", "a/b", `a\b`, "/tmp/x", `C:\x`, `\\server\share`} {
		t.Run(key, func(t *testing.T) {
			if err := users.Set(key, fileMapUser{}); !errors.Is(err, ErrInvalidFileMapKey) {
				t.Fatalf("Set(%q) error = %v", key, err)
			}
		})
	}
}

func TestFileMapReloadIsAllOrNothing(t *testing.T) {
	users, usersDir := loadBoundFileMapForTest(t)
	if err := users.Set("old", fileMapUser{Name: "Old"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usersDir, "new.json"), []byte(`{"name":"New","age":5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := users.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if _, ok := users.Get("new"); !ok {
		t.Fatal("Reload did not read external file")
	}
	if err := os.WriteFile(filepath.Join(usersDir, "bad.json"), []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := users.All()
	if err := users.Reload(); !errors.Is(err, ErrInvalidFileMapEntry) {
		t.Fatalf("malformed Reload error = %v", err)
	}
	if !reflect.DeepEqual(users.All(), before) {
		t.Fatalf("failed Reload changed state: %#v", users.All())
	}
}

func TestFileMapLoaderSaveRewritesKnownAndPreservesOrphans(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	if err := os.WriteFile(configPath, []byte(`{"name":"app"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatal(err)
	}
	var cfg fileMapApp
	if err := loader.LoadInto(&cfg); err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if err := cfg.Users.Set("known", fileMapUser{Name: "Known"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	usersDir := filepath.Join(dir, "users")
	if err := os.WriteFile(filepath.Join(usersDir, "orphan.json"), []byte(`{"name":"Orphan"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(usersDir, "known.json")); err != nil {
		t.Fatal(err)
	}
	if err := loader.Save(&cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	assertFileMapUserFile(t, filepath.Join(usersDir, "known.json"), fileMapUser{Name: "Known"})
	if _, err := os.Stat(filepath.Join(usersDir, "orphan.json")); err != nil {
		t.Fatalf("orphan removed: %v", err)
	}
	var main map[string]any
	readJSONFile(t, configPath, &main)
	if main["users"] != "file://users" {
		t.Fatalf("users ref = %#v", main["users"])
	}
}

type yamlFileMapApp struct {
	Users FileMap[fileMapUser] `yaml:"users" config:"dir=users,format=yaml"`
}

func TestFileMapLoadsYAMLEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("users: file://users/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users", "one.yaml"), []byte("name: Alice\nage: 30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGeneric(func() *yamlFileMapApp { return &yamlFileMapApp{} }, WithConfigPath(filepath.Join(dir, "config.yaml")), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if user, ok := got.Users.Get("one"); !ok || user.Name != "Alice" || user.Age != 30 {
		t.Fatalf("user = %#v, %v", user, ok)
	}
}

type secretFileMapEntry struct {
	Token SecureString `json:"token"`
}
type secretFileMapApp struct {
	Entries FileMap[secretFileMapEntry] `json:"entries" config:"dir=entries"`
}

func TestFileMapRejectsSecretBearingEntryType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"entries":"file://entries/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadGeneric(func() *secretFileMapApp { return &secretFileMapApp{} }, WithConfigPath(filepath.Join(dir, "config.json")), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if !errors.Is(err, ErrInvalidFileMapEntry) {
		t.Fatalf("error = %v, want ErrInvalidFileMapEntry", err)
	}
}

type nestedFileMapEntry struct {
	Children FileMap[fileMapUser] `json:"children"`
}
type nestedFileMapApp struct {
	Entries FileMap[nestedFileMapEntry] `json:"entries" config:"dir=entries"`
}

func TestFileMapRejectsNestedFileMapEntryType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"entries":"file://entries/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadGeneric(func() *nestedFileMapApp { return &nestedFileMapApp{} }, WithConfigPath(filepath.Join(dir, "config.json")), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if !errors.Is(err, ErrInvalidFileMapEntry) {
		t.Fatalf("error = %v, want ErrInvalidFileMapEntry", err)
	}
}

func TestSaveBindsNewEmptyFileMap(t *testing.T) {
	dir := t.TempDir()
	cfg := &fileMapApp{Name: "new", Users: NewFileMap[fileMapUser]()}
	err := SaveGeneric(cfg, WithConfigPath(filepath.Join(dir, "config.json")), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}
	var main map[string]any
	readJSONFile(t, filepath.Join(dir, "config.json"), &main)
	if main["users"] != "file://users" {
		t.Fatalf("users ref = %#v", main["users"])
	}
}

func loadBoundFileMapForTest(t *testing.T) (*FileMap[fileMapUser], string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"users":"file://users/"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGeneric(func() *fileMapApp { return &fileMapApp{} }, WithConfigPath(configPath), WithSecurityPath(filepath.Join(dir, ".security.yml")), WithEnvironment(false))
	if err != nil {
		t.Fatal(err)
	}
	return &got.Users, filepath.Join(dir, "users")
}

func assertFileMapUserFile(t *testing.T, path string, want fileMapUser) {
	t.Helper()
	var got fileMapUser
	readJSONFile(t, path, &got)
	if got != want {
		t.Fatalf("entry = %#v, want %#v", got, want)
	}
}

func TestFileMapPublicReadAPIAndUnboundWrites(t *testing.T) {
	users := NewFileMap[fileMapUser]()
	if users.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", users.Len())
	}
	if _, ok := users.Get("missing"); ok {
		t.Fatal("Get(missing) found value")
	}
	if got := users.Keys(); len(got) != 0 {
		t.Fatalf("Keys() = %v", got)
	}
	copyMap := users.All()
	copyMap["outside"] = fileMapUser{Name: "outside"}
	if users.Len() != 0 {
		t.Fatal("All() exposed internal map")
	}

	for name, err := range map[string]error{
		"Add":    users.Add("one", fileMapUser{Name: "one"}),
		"Update": users.Update("one", fileMapUser{Name: "one"}),
		"Set":    users.Set("one", fileMapUser{Name: "one"}),
		"Delete": users.Delete("one"),
		"Reload": users.Reload(),
	} {
		if !errors.Is(err, ErrFileMapUnbound) {
			t.Fatalf("%s error = %v, want ErrFileMapUnbound", name, err)
		}
	}
	if !reflect.DeepEqual(users.Keys(), []string{}) {
		t.Fatalf("Keys() = %#v", users.Keys())
	}
}
