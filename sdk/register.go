package sdk

var (
	registerFunc func(name string, factory AdapterFactory)
	pendingQueue []pendingRegistration
)

type pendingRegistration struct {
	name    string
	factory AdapterFactory
}

// Register registers an adapter factory. It is safe to call from init().
func Register(name string, factory AdapterFactory) {
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
		panic("sdk.SetRegisterFunc called twice")
	}
	registerFunc = f
	for _, registration := range pendingQueue {
		f(registration.name, registration.factory)
	}
	pendingQueue = nil
}
