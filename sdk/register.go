package sdk

import (
	"sort"
	"strings"
)

var (
	registerFunc    func(name string, factory AdapterFactory)
	pendingQueue    []pendingRegistration
	registeredNames = map[string]struct{}{}
)

type pendingRegistration struct {
	name    string
	factory AdapterFactory
}

// Register registers an adapter factory. It is safe to call from init().
func Register(name string, factory AdapterFactory) {
	recordRegisteredName(name)
	if registerFunc != nil {
		registerFunc(name, factory)
		return
	}
	pendingQueue = append(pendingQueue, pendingRegistration{
		name:    name,
		factory: factory,
	})
}

// SetRegisterFunc wires the kernel registry and drains queued registrations.
func SetRegisterFunc(f func(name string, factory AdapterFactory)) {
	if registerFunc != nil {
		return
	}
	registerFunc = f
	for _, registration := range pendingQueue {
		f(registration.name, registration.factory)
	}
	pendingQueue = nil
}

func RegisteredAdapterNames() []string {
	names := make([]string, 0, len(registeredNames))
	for name := range registeredNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func recordRegisteredName(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	registeredNames[name] = struct{}{}
}
