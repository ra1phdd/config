// Package path provides configuration field path operations.
package path

import "strings"

// Join appends name to parent using the configuration path separator.
func Join(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// Within reports whether child is parent or a descendant of parent.
func Within(child, parent string) bool {
	return child == parent || parent != "" && strings.HasPrefix(child, parent+".")
}

// Origin returns the nearest source origin for path, walking through parents.
func Origin(value string, origins map[string]string) string {
	for current := value; ; {
		if source := origins[current]; source != "" {
			return source
		}
		index := strings.LastIndexByte(current, '.')
		if index < 0 {
			return origins[""]
		}
		current = current[:index]
	}
}
