package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	env "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

func decodeConfigFileInto(path string, data []byte, target any, strictJSON bool) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".env":
		return decodeEnvFileInto(path, target)
	case ".json":
		return decodeJSONInto(data, target, strictJSON)
	case ".yaml", ".yml":
		return decodeYAMLInto(data, target)
	default:
		return fmt.Errorf("%w: unsupported config extension %q", ErrInvalidConfig, filepath.Ext(path))
	}
}

func encodeConfigFile(path string, cfg any) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			return nil, fmt.Errorf("marshal json config: %w", err)
		}
		return buf.Bytes(), nil
	case ".yaml", ".yml":
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(cfg); err != nil {
			_ = enc.Close()
			return nil, fmt.Errorf("marshal yaml config: %w", err)
		}
		if err := enc.Close(); err != nil {
			return nil, fmt.Errorf("close yaml encoder: %w", err)
		}
		return buf.Bytes(), nil
	case ".env":
		return nil, fmt.Errorf("%w: saving .env config files is not supported", ErrInvalidConfig)
	default:
		return nil, fmt.Errorf("%w: unsupported config extension %q", ErrInvalidConfig, filepath.Ext(path))
	}
}

func decodeEnvFileInto(path string, target any) error {
	values, err := godotenv.Read(path)
	if err != nil {
		return fmt.Errorf("parse env config: %w", err)
	}
	restore := applyTemporaryEnv(values, false)
	defer restore()

	if err := env.Parse(target); err != nil {
		return fmt.Errorf("apply env config: %w", err)
	}
	return nil
}

func applyTemporaryEnv(values map[string]string, override bool) func() {
	type previousValue struct {
		value  string
		exists bool
	}
	previous := make(map[string]previousValue, len(values))

	for key, value := range values {
		old, exists := os.LookupEnv(key)
		previous[key] = previousValue{value: old, exists: exists}
		if override || !exists {
			_ = os.Setenv(key, value)
		}
	}

	return func() {
		for key, old := range previous {
			if old.exists {
				_ = os.Setenv(key, old.value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
}

func decodeJSONInto(data []byte, target any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse json config: %w", err)
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values in config")
		}
		return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return nil
}

func decodeYAMLInto(data []byte, target any) error {
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse yaml config: %w", err)
	}
	return nil
}

func decodeYAMLOverlay(path string, target any, allowSymlinks bool) error {
	data, err := readRegularFile(path, allowSymlinks)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("parse yaml config: %w", err)
	}
	if len(node.Content) == 0 {
		return nil
	}

	filtered, ok := filterSecurityOverlayNode(node.Content[0], target)
	if !ok {
		return nil
	}
	return mergeYAMLOverlayNodeIntoTarget(filtered, target)
}

func encodeYAMLOverlay(cfg any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("marshal security config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close security encoder: %w", err)
	}
	return buf.Bytes(), nil
}

func mergeYAMLOverlayNodeIntoTarget(node *yaml.Node, target any) error {
	if target == nil {
		return ErrNilConfig
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errors.New("target must be a non-nil pointer")
	}
	return mergeYAMLNodeIntoValue(node, value.Elem())
}
