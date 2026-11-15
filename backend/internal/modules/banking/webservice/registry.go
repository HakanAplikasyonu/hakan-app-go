package webservice

import "unicode"

var trCase = unicode.TurkishCase

// Environment of a connection: the bank's test system or production.
const (
	EnvTest       = "test"
	EnvProduction = "production"
)

// Factory builds a bank's adapter for an environment.
type Factory func(environment string) Adapter

// Registry maps bank ids to adapters. Only banks with a written adapter are
// offered for "banka hesabını bağla"; the others stay on statement upload.
type Registry map[string]Factory

// Get returns bankID's adapter for environment.
func (r Registry) Get(bankID, environment string) (Adapter, bool) {
	f, ok := r[bankID]
	if !ok {
		return nil, false
	}
	return f(environment), true
}
