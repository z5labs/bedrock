// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package config provides very easy to use and extensible configuration management capabilities.
package config

// Store represents a general key value structure.
type Store interface {
	Set(string, any) error
}

// Source defines valid config sources as those who can
// serialize themselves into a key value like structure.
type Source interface {
	Apply(Store) error
}

// Manager
type Manager struct {
}

// Read
func Read(srcs ...Source) (*Manager, error) {
	return nil, nil
}

// Unmarshal
func (m *Manager) Unmarshal(v any) error {
	return nil
}
