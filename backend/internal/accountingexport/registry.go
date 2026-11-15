package accountingexport

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Factory builds a driver from its configuration.
type Factory func(cfg Config) (Driver, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// ErrUnknownDriver is returned by Open for a name nobody registered.
var ErrUnknownDriver = errors.New("accountingexport: unknown driver")

// Register makes a driver available by name. It panics on an empty name, a
// nil factory or a duplicate name, like database/sql.Register.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" || factory == nil {
		panic("accountingexport: Register needs a name and a factory")
	}
	if _, dup := factories[name]; dup {
		panic("accountingexport: Register called twice for driver " + name)
	}
	factories[name] = factory
}

// Open builds the named driver.
func Open(name string, cfg Config) (Driver, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownDriver, name)
	}
	return f(cfg)
}

// Drivers lists registered driver names, sorted.
func Drivers() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// unregister is for tests.
func unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(factories, name)
}
