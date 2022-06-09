package types

// Provider is an object capable of returning mirror URLs.
type Provider interface {
	// Mirror returns the URL for a mirror.
	// Mirror will be called by pool.Pool workers at any point in time, potentially quite often. Mirror should return
	// reasonably different mirrors on each call, in no particular order.
	Mirror() (string, error)
}

// Builder contains two functions needed for server.Server to build a provider.
type Builder struct {
	// DefaultConfig is expected to return a pointer to an empty struct, which is a provider-specific config.
	// yaml.Unmarshal will be caled on the returned value of this function to deserialize user-defined config into it.
	DefaultConfig func() interface{}

	// New will be called by server.Server to create the provider. The value returned by DefaultConfig will be supplied,
	// after the user-supplied config has ben unmarshalled into it.
	New func(interface{}) (Provider, error)
}
