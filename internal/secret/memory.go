package secret

import "sync"

type memory struct {
	mu     sync.Mutex
	values map[string]string
}

// NewMemory answers a store that forgets everything when the process ends.
func NewMemory() Store { return &memory{values: map[string]string{}} }

func (m *memory) Get(name string) (string, bool, error) {
	if err := checkName(name); err != nil {
		return "", false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	value, found := m.values[name]
	return value, found, nil
}

func (m *memory) Set(name, value string) error {
	if err := checkName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[name] = value
	return nil
}

func (m *memory) Delete(name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, name)
	return nil
}

func (m *memory) Kind() string { return KindMemory }
