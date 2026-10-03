package persist

import (
	"errors"
	"sync"
)

type compareAndDeleter interface {
	CompareAndDelete(key, expected string) (bool, error)
}

type evalStore interface {
	Eval(script, key string, args ...any) (any, error)
}

// Store is the process-wide blob store. Tests keep the memory default.
// The BFF process switches it to Redis in dao.New.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

type mem struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *mem) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		return "", nil
	}
	return s.m[key], nil
}

func (s *mem) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

func (s *mem) CompareAndDelete(key, expected string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[key] != expected {
		return false, nil
	}
	delete(s.m, key)
	return true, nil
}

// Default is memory until Use points it at Redis.
var Default Store = &mem{}

// Use replaces Default. Nil is ignored so tests stay on memory.
func Use(s Store) {
	if s != nil {
		Default = s
	}
}

func CompareAndDelete(key, expected string) (bool, error) {
	s, ok := Default.(compareAndDeleter)
	if ok {
		return s.CompareAndDelete(key, expected)
	}
	if redis, ok := Default.(evalStore); ok {
		v, err := redis.Eval(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`, key, expected)
		if err != nil {
			return false, err
		}
		switch n := v.(type) {
		case int:
			return n == 1, nil
		case int64:
			return n == 1, nil
		default:
			return false, errors.New("unexpected Redis compare-and-delete result")
		}
	}
	return false, nil
}
