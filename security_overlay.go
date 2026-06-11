package config

import (
	"maps"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	secureStringType    = reflect.TypeFor[SecureString]()
	secureStringsType   = reflect.TypeFor[SecureStrings]()
	yamlMarshalerType   = reflect.TypeFor[yaml.Marshaler]()
	yamlUnmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()
)

func extractSecurityOverlay(input any) (any, bool) {
	if input == nil {
		return nil, false
	}
	return extractSecurityOverlayValue(reflect.ValueOf(input))
}

func extractSecurityOverlayValue(v reflect.Value) (any, bool) {
	if !v.IsValid() {
		return nil, false
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		if v.Type().Implements(yamlMarshalerType) && containsSecurityValue(v) {
			return v.Interface(), true
		}
		v = v.Elem()
	}

	if v.Type() == secureStringType {
		secure := v.Interface().(SecureString)
		if secure.String() == "" {
			return nil, false
		}
		if v.CanAddr() {
			return v.Addr().Interface(), true
		}
		return &secure, true
	}

	if v.Type() == secureStringsType {
		secure := v.Interface().(SecureStrings)
		if len(secure.Values()) == 0 {
			return nil, false
		}
		if v.CanAddr() {
			return v.Addr().Interface(), true
		}
		return &secure, true
	}

	if v.Type().Implements(yamlMarshalerType) && containsSecurityValue(v) {
		return v.Interface(), true
	}
	if v.CanAddr() && v.Addr().Type().Implements(yamlMarshalerType) && containsSecurityValue(v) {
		return v.Addr().Interface(), true
	}

	switch v.Kind() {
	case reflect.Struct:
		return extractStructSecurityOverlay(v)
	case reflect.Slice, reflect.Array:
		return extractSequenceSecurityOverlay(v)
	case reflect.Map:
		return extractMapSecurityOverlay(v)
	default:
		return nil, false
	}
}

func extractStructSecurityOverlay(v reflect.Value) (any, bool) {
	t := v.Type()
	out := make(map[string]any)
	for i := range v.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name, inline, skip := yamlFieldName(field)
		if skip {
			continue
		}

		overlay, ok := extractSecurityOverlayValue(v.Field(i))
		if !ok {
			continue
		}

		if inline {
			if inlineMap, ok := overlay.(map[string]any); ok {
				maps.Copy(out, inlineMap)
			}
			continue
		}
		out[name] = overlay
	}
	return out, len(out) > 0
}

func extractSequenceSecurityOverlay(v reflect.Value) (any, bool) {
	out := make([]any, 0, v.Len())
	anyValue := false
	for i := range v.Len() {
		item := v.Index(i)
		overlay, ok := extractSecurityOverlayValue(item)
		if ok {
			anyValue = true
			overlay = attachMergeKey(item, overlay)
			out = append(out, overlay)
			continue
		}
		out = append(out, nil)
	}
	if !anyValue {
		return nil, false
	}
	return out, true
}

func extractMapSecurityOverlay(v reflect.Value) (any, bool) {
	if v.Len() == 0 {
		return nil, false
	}

	out := make(map[any]any)
	for _, key := range v.MapKeys() {
		overlay, ok := extractSecurityOverlayValue(v.MapIndex(key))
		if !ok {
			continue
		}
		out[key.Interface()] = overlay
	}
	return out, len(out) > 0
}

func containsSecurityValue(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}

	if v.Type() == secureStringType {
		secure := v.Interface().(SecureString)
		return secure.String() != ""
	}
	if v.Type() == secureStringsType {
		secure := v.Interface().(SecureStrings)
		return len(secure.Values()) > 0
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if t.Field(i).IsExported() && containsSecurityValue(v.Field(i)) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if containsSecurityValue(v.Index(i)) {
				return true
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if containsSecurityValue(v.MapIndex(key)) {
				return true
			}
		}
	}
	return false
}

func yamlFieldName(field reflect.StructField) (name string, inline bool, skip bool) {
	tag := field.Tag.Get("yaml")
	if tag == "-" {
		return "", false, true
	}

	parts := strings.Split(tag, ",")
	if parts[0] != "" {
		name = parts[0]
	} else {
		name = strings.ToLower(field.Name)
	}
	for _, part := range parts[1:] {
		if part == "inline" {
			inline = true
		}
	}
	return name, inline, false
}

func filterSecurityOverlayNode(node *yaml.Node, target any) (*yaml.Node, bool) {
	if node == nil || target == nil {
		return nil, false
	}
	filtered := *node
	ok := filterSecurityOverlayNodeValue(&filtered, reflect.TypeOf(target))
	if !ok {
		return nil, false
	}
	return &filtered, true
}

func filterSecurityOverlayNodeValue(node *yaml.Node, t reflect.Type) bool {
	if node == nil || t == nil {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if isSecureType(t) || typeHandlesSecurityOverlay(t) {
		return true
	}

	switch t.Kind() {
	case reflect.Struct:
		return filterStructSecurityOverlayNode(node, t)
	case reflect.Slice, reflect.Array:
		return filterSequenceSecurityOverlayNode(node, t.Elem())
	case reflect.Map:
		return filterMapSecurityOverlayNode(node, t.Elem())
	default:
		return false
	}
}

func filterStructSecurityOverlayNode(node *yaml.Node, t reflect.Type) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}

	fields := securityYAMLFields(t)
	out := make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		field, ok := fields[key.Value]
		if !ok {
			continue
		}
		filteredValue := *value
		if !filterSecurityOverlayNodeValue(&filteredValue, field.Type) {
			continue
		}
		out = append(out, key, &filteredValue)
	}
	node.Content = out
	return len(out) > 0
}

func filterSequenceSecurityOverlayNode(node *yaml.Node, elem reflect.Type) bool {
	if node.Kind != yaml.SequenceNode {
		return filterSecurityOverlayNodeValue(node, elem)
	}

	out := make([]*yaml.Node, 0, len(node.Content))
	for _, child := range node.Content {
		filteredChild := *child
		if filterSecurityOverlayNodeValue(&filteredChild, elem) {
			attachMergeKeyNode(&filteredChild, child, elem)
			out = append(out, &filteredChild)
		}
	}
	node.Content = out
	return len(out) > 0
}

func filterMapSecurityOverlayNode(node *yaml.Node, elem reflect.Type) bool {
	if node.Kind != yaml.MappingNode {
		return filterSecurityOverlayNodeValue(node, elem)
	}

	out := make([]*yaml.Node, 0, len(node.Content))
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		filteredValue := *value
		if !filterSecurityOverlayNodeValue(&filteredValue, elem) {
			continue
		}
		out = append(out, key, &filteredValue)
	}
	node.Content = out
	return len(out) > 0
}

func securityYAMLFields(t reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField)
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, inline, skip := yamlFieldName(field)
		if skip {
			continue
		}
		if inline {
			inlineType := field.Type
			for inlineType.Kind() == reflect.Pointer {
				inlineType = inlineType.Elem()
			}
			if inlineType.Kind() == reflect.Struct {
				maps.Copy(fields, securityYAMLFields(inlineType))
			}
			continue
		}
		fields[name] = field
	}
	return fields
}

func isSecureType(t reflect.Type) bool {
	return t == secureStringType || t == secureStringsType
}

func typeHandlesSecurityOverlay(t reflect.Type) bool {
	return (t.Implements(yamlMarshalerType) || reflect.PointerTo(t).Implements(yamlMarshalerType) ||
		t.Implements(yamlUnmarshalerType) || reflect.PointerTo(t).Implements(yamlUnmarshalerType)) &&
		typeContainsSecurity(t, make(map[reflect.Type]bool))
}

func typeContainsSecurity(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface {
		return false
	}
	if isSecureType(t) {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true

	switch t.Kind() {
	case reflect.Struct:
		for i := range t.NumField() {
			field := t.Field(i)
			if field.IsExported() && typeContainsSecurity(field.Type, seen) {
				return true
			}
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		return typeContainsSecurity(t.Elem(), seen)
	}
	return false
}

func attachMergeKey(v reflect.Value, overlay any) any {
	if overlay == nil {
		return nil
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return overlay
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return overlay
	}
	field, yamlName, ok := mergeKeyField(v.Type())
	if !ok {
		return overlay
	}
	if value := strings.TrimSpace(v.FieldByIndex(field.Index).String()); value != "" {
		if m, ok := overlay.(map[string]any); ok {
			if _, exists := m[yamlName]; !exists {
				m[yamlName] = value
			}
		}
	}
	return overlay
}

func attachMergeKeyNode(dst *yaml.Node, src *yaml.Node, t reflect.Type) {
	if dst == nil || src == nil {
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}
	_, yamlName, ok := mergeKeyField(t)
	if !ok {
		return
	}
	if hasYAMLMappingKey(dst, yamlName) {
		return
	}
	key, value, found := findYAMLMappingPair(src, yamlName)
	if !found {
		return
	}
	dst.Content = append(dst.Content, cloneYAMLNode(key), cloneYAMLNode(value))
}

func mergeKeyField(t reflect.Type) (reflect.StructField, string, bool) {
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.String {
			continue
		}
		name, _, skip := yamlFieldName(field)
		if skip {
			continue
		}
		if name == "name" {
			return field, name, true
		}
	}
	return reflect.StructField{}, "", false
}

func hasYAMLMappingKey(node *yaml.Node, key string) bool {
	_, _, found := findYAMLMappingPair(node, key)
	return found
}

func findYAMLMappingPair(node *yaml.Node, key string) (*yaml.Node, *yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i], node.Content[i+1], true
		}
	}
	return nil, nil, false
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, 0, len(node.Content))
		for _, child := range node.Content {
			cloned.Content = append(cloned.Content, cloneYAMLNode(child))
		}
	}
	return &cloned
}
