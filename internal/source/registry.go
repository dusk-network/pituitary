package source

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/sdk"
)

var registry = map[string]sdk.AdapterFactory{}

func init() {
	sdk.SetRegisterFunc(Register)
}

func Register(name string, factory sdk.AdapterFactory) {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("source adapter name is required")
	}
	if factory == nil {
		panic(fmt.Sprintf("source adapter %q factory is nil", name))
	}
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("source adapter %q already registered", name))
	}
	registry[name] = factory
}

func LookupAdapter(name string) sdk.AdapterFactory {
	return registry[strings.TrimSpace(name)]
}

func RegisteredAdapters() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
