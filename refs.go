package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

type fileRefSpec struct {
	fieldIndex  []int
	jsonPath    []string
	yamlPath    []string
	defaultPath string
}

type fileRefBinding struct {
	spec fileRefSpec
	ref  string
}

func loadConfigWithRefs(path string, data []byte, target any, allowSymlinks bool, strictJSON bool, resolver PathResolver) error {
	specs := discoverFileRefSpecs(reflect.TypeOf(target))
	if len(specs) == 0 {
		return decodeConfigFileInto(path, data, target, strictJSON)
	}

	patched := data
	bindings := make([]fileRefBinding, 0, len(specs))
	var err error

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		patched, bindings, err = extractJSONFileRefs(data, specs)
	case ".yaml", ".yml":
		patched, bindings, err = extractYAMLFileRefs(data, specs)
	default:
		return decodeConfigFileInto(path, data, target, strictJSON)
	}
	if err != nil {
		return err
	}

	if err := decodeConfigFileInto(path, patched, target, strictJSON); err != nil {
		return err
	}

	return applyFileRefBindings(path, bindings, target, allowSymlinks, strictJSON, resolver)
}

func encodeConfigWithRefs(path string, cfg any, allowSymlinks bool, resolver PathResolver) ([]byte, error) {
	if err := syncDirectoryFileMaps(cfg); err != nil {
		return nil, err
	}
	data, err := encodeConfigFile(path, cfg)
	if err != nil {
		return nil, err
	}

	specs := discoverFileRefSpecs(reflect.TypeOf(cfg))
	if len(specs) == 0 {
		return data, nil
	}

	bindings := discoverConfiguredFileRefs(path, specs, resolver)
	if len(bindings) == 0 {
		return data, nil
	}

	if err := writeFileRefTargets(path, bindings, cfg, allowSymlinks, resolver); err != nil {
		return nil, err
	}

	return replaceEncodedFileRefs(path, data, bindings)
}

func discoverFileRefSpecs(t reflect.Type) []fileRefSpec {
	var specs []fileRefSpec
	collectFileRefSpecs(indirectType(t), nil, nil, nil, &specs)
	return specs
}

func collectFileRefSpecs(t reflect.Type, fieldIndex []int, jsonPath []string, yamlPath []string, specs *[]fileRefSpec) {
	if t == nil || t.Kind() != reflect.Struct {
		return
	}

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		refPath, isFileRef := parseConfigTag(field.Tag.Get("config"))
		fieldIndexPath := append(append([]int(nil), fieldIndex...), field.Index...)
		jsonName, skipJSON := jsonFieldName(field)
		yamlName, yamlInline, skipYAML := yamlFieldName(field)
		fieldJSONPath, canJSON := appendFieldPath(jsonPath, jsonName, false, skipJSON)
		fieldYAMLPath, canYAML := appendFieldPath(yamlPath, yamlName, yamlInline, skipYAML)

		if isFileRef {
			*specs = append(*specs, fileRefSpec{
				fieldIndex:  fieldIndexPath,
				jsonPath:    fieldJSONPath,
				yamlPath:    fieldYAMLPath,
				defaultPath: refPath,
			})
			continue
		}

		nextType := indirectType(field.Type)
		if nextType.Kind() != reflect.Struct {
			continue
		}
		if canJSON || canYAML {
			collectFileRefSpecs(nextType, fieldIndexPath, fieldJSONPath, fieldYAMLPath, specs)
		}
	}
}

func parseConfigTag(tag string) (string, bool) {
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "file":
			return "", true
		case strings.HasPrefix(part, "file="):
			return strings.TrimSpace(strings.TrimPrefix(part, "file=")), true
		}
	}
	return "", false
}

func appendFieldPath(parent []string, name string, inline bool, skip bool) ([]string, bool) {
	if skip {
		return nil, false
	}
	if inline {
		return append([]string(nil), parent...), true
	}
	if name == "" {
		return nil, false
	}
	return append(append([]string(nil), parent...), name), true
}

func jsonFieldName(field reflect.StructField) (name string, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		return parts[0], false
	}
	return strings.ToLower(field.Name), false
}

func extractJSONFileRefs(data []byte, specs []fileRefSpec) ([]byte, []fileRefBinding, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parse json config: %w", err)
	}

	bindings := make([]fileRefBinding, 0, len(specs))
	for _, spec := range specs {
		if len(spec.jsonPath) == 0 {
			continue
		}
		if ref, ok := findJSONFileRef(root, spec.jsonPath); ok {
			bindings = append(bindings, fileRefBinding{spec: spec, ref: ref})
			setJSONPathValue(&root, spec.jsonPath, nil, false)
		}
	}
	if len(bindings) == 0 {
		return data, nil, nil
	}

	patched, err := json.Marshal(root)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal json config: %w", err)
	}
	return patched, bindings, nil
}

func extractYAMLFileRefs(data []byte, specs []fileRefSpec) ([]byte, []fileRefBinding, error) {
	root, err := decodeYAMLDocument(data)
	if err != nil {
		return nil, nil, err
	}

	bindings := make([]fileRefBinding, 0, len(specs))
	for _, spec := range specs {
		if len(spec.yamlPath) == 0 {
			continue
		}
		node := findYAMLPathNode(root, spec.yamlPath)
		if node == nil || node.Kind != yaml.ScalarNode {
			continue
		}
		if ref, ok := fileReferenceValue(node.Value); ok {
			bindings = append(bindings, fileRefBinding{spec: spec, ref: ref})
			*node = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
		}
	}
	if len(bindings) == 0 {
		return data, nil, nil
	}

	patched, err := encodeYAMLDocument(root)
	if err != nil {
		return nil, nil, err
	}
	return patched, bindings, nil
}

func applyFileRefBindings(configPath string, bindings []fileRefBinding, target any, allowSymlinks bool, strictJSON bool, resolver PathResolver) error {
	if len(bindings) == 0 {
		return nil
	}

	root := reflect.ValueOf(target)
	if root.Kind() != reflect.Pointer || root.IsNil() {
		return ErrNilConfig
	}

	for _, binding := range bindings {
		field := root.Elem().FieldByIndex(binding.spec.fieldIndex)
		if !field.CanSet() {
			continue
		}
		if err := decodeFileRefIntoValue(configPath, binding.ref, field, allowSymlinks, strictJSON, resolver); err != nil {
			return err
		}
	}
	return nil
}

func decodeFileRefIntoValue(configPath string, ref string, dst reflect.Value, allowSymlinks bool, strictJSON bool, resolver PathResolver) error {
	path, err := resolveFileReferencePath(configPath, ref, resolver)
	if err != nil {
		return err
	}
	data, err := readRegularFile(path, allowSymlinks)
	if err != nil {
		return fmt.Errorf("read file ref %q: %w", ref, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}

	holder := reflect.New(dst.Type())
	if err := decodeConfigFileInto(path, data, holder.Interface(), strictJSON); err != nil {
		return fmt.Errorf("decode file ref %q: %w", ref, err)
	}
	dst.Set(holder.Elem())
	return nil
}

func discoverConfiguredFileRefs(configPath string, specs []fileRefSpec, resolver PathResolver) []fileRefBinding {
	configured := make(map[string]string, len(specs))
	if data, err := os.ReadFile(configPath); err == nil {
		configured = readConfiguredFileRefs(configPath, data, specs)
	}

	bindings := make([]fileRefBinding, 0, len(specs))
	for _, spec := range specs {
		key := specKey(spec)
		ref := strings.TrimSpace(configured[key])
		if ref == "" {
			ref = strings.TrimSpace(spec.defaultPath)
		}
		if ref == "" {
			continue
		}
		if !strings.HasPrefix(ref, FileScheme) {
			ref = FileScheme + ref
		}
		bindings = append(bindings, fileRefBinding{spec: spec, ref: ref})
	}
	return bindings
}

func readConfiguredFileRefs(configPath string, data []byte, specs []fileRefSpec) map[string]string {
	configured := make(map[string]string, len(specs))
	switch strings.ToLower(filepath.Ext(configPath)) {
	case ".json":
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			return configured
		}
		for _, spec := range specs {
			if ref, ok := findJSONFileRef(root, spec.jsonPath); ok {
				configured[specKey(spec)] = ref
			}
		}
	case ".yaml", ".yml":
		root, err := decodeYAMLDocument(data)
		if err != nil {
			return configured
		}
		for _, spec := range specs {
			node := findYAMLPathNode(root, spec.yamlPath)
			if node == nil {
				continue
			}
			if ref, ok := fileReferenceValue(node.Value); ok {
				configured[specKey(spec)] = ref
			}
		}
	}
	return configured
}

func writeFileRefTargets(configPath string, bindings []fileRefBinding, cfg any, allowSymlinks bool, resolver PathResolver) error {
	root := reflect.ValueOf(cfg)
	if root.Kind() == reflect.Pointer {
		root = root.Elem()
	}

	for _, binding := range bindings {
		field := root.FieldByIndex(binding.spec.fieldIndex)
		path, err := resolveFileReferencePath(configPath, binding.ref, resolver)
		if err != nil {
			return err
		}
		data, err := encodeConfigFile(path, field.Interface())
		if err != nil {
			return fmt.Errorf("encode file ref %q: %w", binding.ref, err)
		}
		if err := writePrivateFile(path, data, allowSymlinks); err != nil {
			return fmt.Errorf("save file ref %q: %w", binding.ref, err)
		}
	}
	return nil
}

func replaceEncodedFileRefs(configPath string, data []byte, bindings []fileRefBinding) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(configPath)) {
	case ".json":
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("parse json config: %w", err)
		}
		for _, binding := range bindings {
			setJSONPathValue(&root, binding.spec.jsonPath, binding.ref, true)
		}
		encoded, err := marshalIndentedJSON(root)
		if err != nil {
			return nil, err
		}
		return encoded, nil
	case ".yaml", ".yml":
		root, err := decodeYAMLDocument(data)
		if err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			setYAMLPathValue(root, binding.spec.yamlPath, binding.ref)
		}
		return encodeYAMLDocument(root)
	default:
		return data, nil
	}
}

func findJSONFileRef(root any, path []string) (string, bool) {
	current := root
	for i, part := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		value, exists := mapping[part]
		if !exists {
			return "", false
		}
		if i == len(path)-1 {
			text, ok := value.(string)
			if !ok {
				return "", false
			}
			return fileReferenceValue(text)
		}
		current = value
	}
	return "", false
}

func setJSONPathValue(root *any, path []string, value any, createMissing bool) {
	if root == nil || len(path) == 0 {
		return
	}

	mapping, ok := (*root).(map[string]any)
	if !ok {
		mapping = map[string]any{}
		*root = mapping
	}

	current := mapping
	for i, part := range path[:len(path)-1] {
		next, exists := current[part]
		nextMap, ok := next.(map[string]any)
		if !exists || !ok {
			if !createMissing && i == 0 && !exists {
				return
			}
			nextMap = map[string]any{}
			current[part] = nextMap
		}
		current = nextMap
	}
	current[path[len(path)-1]] = value
}

func findYAMLPathNode(root *yaml.Node, path []string) *yaml.Node {
	current := root
	for _, part := range path {
		if current == nil || current.Kind != yaml.MappingNode {
			return nil
		}
		_, next, found := findYAMLMappingPair(current, part)
		if !found {
			return nil
		}
		current = next
	}
	return current
}

func setYAMLPathValue(root *yaml.Node, path []string, value string) {
	if root == nil || len(path) == 0 {
		return
	}

	current := root
	for _, part := range path[:len(path)-1] {
		if current.Kind == 0 {
			current.Kind = yaml.MappingNode
			current.Tag = "!!map"
		}
		_, next, found := findYAMLMappingPair(current, part)
		if !found {
			key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part}
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, key, next)
		}
		current = next
	}
	key, next, found := findYAMLMappingPair(current, path[len(path)-1])
	if !found {
		key = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[len(path)-1]}
		next = &yaml.Node{}
		current.Content = append(current.Content, key, next)
	}
	_ = key
	*next = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func fileReferenceValue(raw string) (string, bool) {
	ref := strings.TrimSpace(raw)
	if !strings.HasPrefix(ref, FileScheme) {
		return "", false
	}
	path := strings.TrimSpace(strings.TrimPrefix(ref, FileScheme))
	if path == "" || strings.HasPrefix(path, FileScheme) {
		return "", false
	}
	return ref, true
}

func resolveFileReferencePath(configPath string, ref string, resolver PathResolver) (string, error) {
	path, ok := fileReferenceValue(ref)
	if !ok {
		return "", fmt.Errorf("invalid file reference %q", ref)
	}
	baseDir := filepath.Dir(configPath)
	return resolver.ResolveAgainst(strings.TrimPrefix(path, FileScheme), baseDir), nil
}

func specKey(spec fileRefSpec) string {
	if len(spec.jsonPath) > 0 {
		return strings.Join(spec.jsonPath, ".")
	}
	return strings.Join(spec.yamlPath, ".")
}

func indirectType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func decodeYAMLDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml config: %w", err)
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	return doc.Content[0], nil
}

func encodeYAMLDocument(root *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("marshal yaml config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

func marshalIndentedJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("marshal json config: %w", err)
	}
	return buf.Bytes(), nil
}
