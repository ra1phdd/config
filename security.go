package config

import (
	"fmt"
	"os"
)

func loadYAMLOverlay(target any, path string, allowSymlinks bool) error {
	if target == nil {
		return ErrNilConfig
	}
	if err := decodeYAMLOverlay(path, target, allowSymlinks); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read security config: %w", err)
	}
	return nil
}

func saveYAMLOverlay(path string, cfg any, allowSymlinks bool) error {
	if cfg == nil {
		return ErrNilConfig
	}
	overlay, _ := extractSecurityOverlay(cfg)
	if overlay == nil {
		overlay = map[string]any{}
	}
	data, err := encodeYAMLOverlay(overlay)
	if err != nil {
		return err
	}
	return writePrivateFile(path, data, allowSymlinks)
}
