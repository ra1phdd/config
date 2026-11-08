package config

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

const securityMergeTag = "securityMerge"

type mergeIdentityField struct {
	field reflect.StructField
	name  string
}

func indirectStructType(t reflect.Type) (reflect.Type, bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t, t.Kind() == reflect.Struct
}

func mergeIdentityFields(t reflect.Type) ([]mergeIdentityField, bool, error) {
	base, ok := indirectStructType(t)
	if !ok {
		return nil, false, nil
	}
	var explicit, automatic []mergeIdentityField
	for i := range base.NumField() {
		f := base.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, skip := yamlFieldName(f)
		if skip {
			continue
		}
		tag, tagged := f.Tag.Lookup(securityMergeTag)
		if tagged && tag != "key" {
			return nil, false, fmt.Errorf("%w: field %s has %s:%q", ErrInvalidMergeKey, f.Name, securityMergeTag, tag)
		}
		if tagged && !mergeScalarKind(f.Type.Kind()) {
			return nil, false, fmt.Errorf("%w: field %s has unsupported type %s", ErrInvalidMergeKey, f.Name, f.Type)
		}
		item := mergeIdentityField{field: f, name: name}
		if tagged {
			explicit = append(explicit, item)
		} else if mergeScalarKind(f.Type.Kind()) {
			automatic = append(automatic, item)
		}
	}
	if len(explicit) > 0 {
		return explicit, true, nil
	}
	return automatic, false, nil
}

func mergeScalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func identityNodes(node *yaml.Node, fields []mergeIdentityField, explicit bool) (map[string]*yaml.Node, error) {
	out := make(map[string]*yaml.Node)
	for _, f := range fields {
		_, value, found := findYAMLMappingPair(node, f.name)
		if !found {
			if explicit {
				return nil, fmt.Errorf("%w: missing field %q", ErrInvalidMergeKey, f.name)
			}
			continue
		}
		out[f.name] = value
	}
	return out, nil
}

func dereferenceValue(v reflect.Value, allocate bool) (reflect.Value, bool) {
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if !allocate || !v.CanSet() {
				return reflect.Value{}, false
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v, v.IsValid()
}

func resolveSequenceCandidate(node *yaml.Node, dst reflect.Value, fields []mergeIdentityField, explicit bool) (int, map[string]*yaml.Node, error) {
	nodes, err := identityNodes(node, fields, explicit)
	if err != nil {
		return -1, nil, err
	}
	var candidates map[int]struct{}
	matchedSelector := false
	for _, f := range fields {
		n, present := nodes[f.name]
		if !present {
			continue
		}
		want := reflect.New(f.field.Type)
		if err := n.Decode(want.Interface()); err != nil {
			return -1, nil, fmt.Errorf("decode selector %q: %w", f.name, err)
		}
		matches := make(map[int]struct{})
		for i := 0; i < dst.Len(); i++ {
			elem, ok := dereferenceValue(dst.Index(i), false)
			if ok && elem.Kind() == reflect.Struct && reflect.DeepEqual(elem.FieldByIndex(f.field.Index).Interface(), want.Elem().Interface()) {
				matches[i] = struct{}{}
			}
		}
		if len(matches) == 0 {
			continue
		}
		matchedSelector = true
		if candidates == nil {
			candidates = matches
			continue
		}
		for idx := range candidates {
			if _, ok := matches[idx]; !ok {
				delete(candidates, idx)
			}
		}
		if len(candidates) == 0 {
			return -1, nil, fmt.Errorf("%w: selectors identify different elements", ErrAmbiguousMerge)
		}
	}
	if !matchedSelector {
		return -1, nodes, nil
	}
	if len(candidates) != 1 {
		return -1, nil, fmt.Errorf("%w: selectors match %d elements", ErrAmbiguousMerge, len(candidates))
	}
	for idx := range candidates {
		return idx, nodes, nil
	}
	return -1, nodes, nil
}

func withoutIdentityNodes(node *yaml.Node, identities map[string]*yaml.Node) *yaml.Node {
	cloned := cloneYAMLNode(node)
	if cloned == nil || cloned.Kind != yaml.MappingNode {
		return cloned
	}
	out := cloned.Content[:0]
	for i := 0; i+1 < len(cloned.Content); i += 2 {
		if _, identity := identities[cloned.Content[i].Value]; identity {
			continue
		}
		out = append(out, cloned.Content[i], cloned.Content[i+1])
	}
	cloned.Content = out
	return cloned
}
