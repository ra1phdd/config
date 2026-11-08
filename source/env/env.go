// Package env provides process and file environment sources.
package env

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/ra1phdd/config"
	fieldenv "github.com/ra1phdd/config/internal/envmap"
	genericSource "github.com/ra1phdd/config/source"
)

// Process reads process environment variables using explicit env tags.
func Process() config.Source {
	return processSource{name: "env", lookup: os.LookupEnv}
}

// File reads an environment file and resolves its keys against env tags. An
// optional name overrides the automatically derived basename.
func File(path string, names ...string) config.Source {
	return genericSource.File(path, Parser{}, names...)
}

type processSource struct {
	name   string
	lookup func(string) (string, bool)
}

func (s processSource) Name() string { return s.name }

func (s processSource) Load(ctx context.Context) (config.Layer, error) {
	if err := ctx.Err(); err != nil {
		return config.Layer{}, fmt.Errorf("checking environment context: %w", err)
	}
	return config.Layer{Name: s.name, Data: map[string]any{}}, nil
}

func (s processSource) LoadFor(ctx context.Context, target reflect.Type) (config.Layer, error) {
	if err := ctx.Err(); err != nil {
		return config.Layer{}, fmt.Errorf("checking environment context: %w", err)
	}
	values, err := fieldenv.ParseStruct(target, s.lookup)
	if err != nil {
		return config.Layer{}, fmt.Errorf("parse environment source: %w", err)
	}
	return config.Layer{Name: s.name, Data: values}, nil
}

// Parser implements config.Parser and config.TargetParser for KEY=value files.
type Parser struct{}

// Parse decodes an environment file without resolving it against a target.
func (Parser) Parse(data []byte) (config.Layer, error) {
	values, err := Parse(data)
	if err != nil {
		return config.Layer{}, err
	}
	return config.Layer{Data: nested(values)}, nil
}

// ParseFor resolves environment-file keys against config and env tags.
func (Parser) ParseFor(data []byte, target reflect.Type, strict bool) (config.Layer, error) {
	values, err := Parse(data)
	if err != nil {
		return config.Layer{}, err
	}
	if strict {
		if err := fieldenv.ValidateKeys(target, values); err != nil {
			return config.Layer{}, fmt.Errorf("validate environment file keys: %w", err)
		}
	}
	parsed, err := fieldenv.ParseStruct(target, func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		return config.Layer{}, fmt.Errorf("map environment file values to fields: %w", err)
	}
	return config.Layer{Data: parsed, Strict: strict}, nil
}

// Parse reads KEY=value lines without changing process state.
func Parse(data []byte) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")
		key, raw, found := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !found || !validName(key) {
			return nil, fmt.Errorf("line %d: expected KEY=value", line)
		}
		value, err := parseValue(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan environment file: %w", err)
	}
	return values, nil
}

func validName(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func parseValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", errors.New("invalid quoted value")
		}
		return raw[1 : len(raw)-1], nil
	case '"':
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", errors.New("invalid quoted value")
		}
		return value, nil
	default:
		if index := strings.Index(raw, " #"); index >= 0 {
			raw = strings.TrimSpace(raw[:index])
		}
		return strings.ReplaceAll(raw, `\#`, "#"), nil
	}
}

func nested(values map[string]string) map[string]any {
	root := make(map[string]any)
	for key, value := range values {
		parts := strings.Split(strings.ToLower(key), "_")
		current := root
		for _, part := range parts[:len(parts)-1] {
			child, ok := current[part].(map[string]any)
			if !ok {
				child = make(map[string]any)
				current[part] = child
			}
			current = child
		}
		current[parts[len(parts)-1]] = value
	}
	return root
}

var (
	_ config.Parser       = Parser{}
	_ config.TargetParser = Parser{}
	_ config.TargetSource = processSource{}
)
