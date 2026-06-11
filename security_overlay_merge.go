package config

import (
	"fmt"
	"reflect"
	"strconv"

	"gopkg.in/yaml.v3"
)

func mergeYAMLNodeIntoValue(node *yaml.Node, dst reflect.Value) error {
	if !dst.IsValid() || node == nil {
		return nil
	}
	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	if implementsYAMLUnmarshaler(dst) || isSecureType(dst.Type()) {
		return decodeNodeIntoValue(node, dst)
	}

	switch dst.Kind() {
	case reflect.Struct:
		return mergeStructNodeIntoValue(node, dst)
	case reflect.Map:
		return mergeMapNodeIntoValue(node, dst)
	case reflect.Slice:
		return mergeSliceNodeIntoValue(node, dst)
	default:
		return decodeNodeIntoValue(node, dst)
	}
}

func mergeStructNodeIntoValue(node *yaml.Node, dst reflect.Value) error {
	if node.Kind != yaml.MappingNode {
		return decodeNodeIntoValue(node, dst)
	}
	fields := securityYAMLFields(dst.Type())
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		field, ok := fields[key]
		if !ok {
			continue
		}
		value := dst.FieldByIndex(field.Index)
		if !value.CanSet() {
			continue
		}
		if err := mergeYAMLNodeIntoValue(node.Content[i+1], value); err != nil {
			return err
		}
	}
	return nil
}

func mergeMapNodeIntoValue(node *yaml.Node, dst reflect.Value) error {
	if node.Kind != yaml.MappingNode {
		return decodeNodeIntoValue(node, dst)
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	elemType := dst.Type().Elem()
	keyType := dst.Type().Key()
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyValue, err := yamlScalarToKey(node.Content[i].Value, keyType)
		if err != nil {
			return err
		}
		current := dst.MapIndex(keyValue)
		var next reflect.Value
		if current.IsValid() {
			next = reflect.New(elemType).Elem()
			next.Set(current)
		} else {
			next = reflect.New(elemType).Elem()
		}
		if err := mergeYAMLNodeIntoValue(node.Content[i+1], next); err != nil {
			return err
		}
		dst.SetMapIndex(keyValue, next)
	}
	return nil
}

func mergeSliceNodeIntoValue(node *yaml.Node, dst reflect.Value) error {
	if node.Kind != yaml.SequenceNode {
		return decodeNodeIntoValue(node, dst)
	}
	elemType := dst.Type().Elem()
	if elemType.Kind() == reflect.Struct {
		if field, yamlName, ok := mergeKeyField(elemType); ok {
			return mergeNamedSliceNodeIntoValue(node, dst, field, yamlName)
		}
	}
	result := reflect.MakeSlice(dst.Type(), 0, len(node.Content))
	for _, child := range node.Content {
		elem := reflect.New(elemType).Elem()
		if err := mergeYAMLNodeIntoValue(child, elem); err != nil {
			return err
		}
		result = reflect.Append(result, elem)
	}
	dst.Set(result)
	return nil
}

func mergeNamedSliceNodeIntoValue(node *yaml.Node, dst reflect.Value, field reflect.StructField, yamlName string) error {
	indexByName := make(map[string]int, dst.Len())
	for i := range dst.Len() {
		name := dst.Index(i).FieldByIndex(field.Index).String()
		if name != "" {
			indexByName[name] = i
		}
	}

	for _, child := range node.Content {
		if child.Kind != yaml.MappingNode {
			continue
		}
		_, keyNode, found := findYAMLMappingPair(child, yamlName)
		if !found {
			continue
		}
		name := keyNode.Value
		if idx, exists := indexByName[name]; exists {
			if err := mergeYAMLNodeIntoValue(child, dst.Index(idx)); err != nil {
				return err
			}
			continue
		}

		elem := reflect.New(dst.Type().Elem()).Elem()
		if err := mergeYAMLNodeIntoValue(child, elem); err != nil {
			return err
		}
		dst.Set(reflect.Append(dst, elem))
		indexByName[name] = dst.Len() - 1
	}
	return nil
}

func decodeNodeIntoValue(node *yaml.Node, dst reflect.Value) error {
	holder := reflect.New(dst.Type())
	if dst.IsValid() && dst.CanInterface() {
		holder.Elem().Set(dst)
	}
	if err := node.Decode(holder.Interface()); err != nil {
		return err
	}
	dst.Set(holder.Elem())
	return nil
}

func yamlScalarToKey(raw string, t reflect.Type) (reflect.Value, error) {
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(raw).Convert(t), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("parse map key %q: %w", raw, err)
		}
		v := reflect.New(t).Elem()
		v.SetInt(n)
		return v, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("parse map key %q: %w", raw, err)
		}
		v := reflect.New(t).Elem()
		v.SetUint(n)
		return v, nil
	default:
		key := reflect.New(t)
		if err := yaml.Unmarshal([]byte(raw), key.Interface()); err != nil {
			return reflect.Value{}, err
		}
		return key.Elem(), nil
	}
}

func implementsYAMLUnmarshaler(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	t := v.Type()
	return t.Implements(yamlUnmarshalerType) || reflect.PointerTo(t).Implements(yamlUnmarshalerType)
}
