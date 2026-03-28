package config

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

func parse(file io.Reader) (rawConfig, error) {
	var cfg rawConfig

	metadata, err := toml.NewDecoder(file).Decode(&cfg)
	if err != nil {
		return rawConfig{}, err
	}

	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return rawConfig{}, formatUndecodedKeys(undecoded)
	}

	return cfg, nil
}

func formatUndecodedKeys(keys []toml.Key) error {
	messages := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		message := undecodedKeyMessage(key)
		if _, exists := seen[message]; exists {
			continue
		}
		seen[message] = struct{}{}
		messages = append(messages, message)
	}
	sort.Strings(messages)
	return errors.New(strings.Join(messages, "\n"))
}

func undecodedKeyMessage(key toml.Key) string {
	if len(key) == 0 {
		return "unsupported empty key"
	}
	if len(key) == 1 {
		return fmt.Sprintf("key %q is outside a supported section", key[0])
	}

	switch key[0] {
	case "workspace":
		return fmt.Sprintf("unsupported workspace field %q", strings.Join(key[1:], "."))
	case "runtime":
		switch len(key) {
		case 2:
			return fmt.Sprintf("unsupported runtime field %q", key[1])
		default:
			return fmt.Sprintf("unsupported runtime.%s field %q", key[1], strings.Join(key[2:], "."))
		}
	case "sources":
		return fmt.Sprintf("unsupported sources field %q", strings.Join(key[1:], "."))
	default:
		return fmt.Sprintf("unsupported section %q", key[0])
	}
}
