package protocol

import (
	"fmt"
	"sync"
)

// Message is the unpacked message object.
type Message struct {
	ID      interface{}
	Path   string
	Method string
	Payload    []byte
	storage map[string]interface{}
	mu      sync.RWMutex
}

// Set stores kv pair.
func (e *Message) Set(key string, value interface{}) {
	e.mu.Lock()
	if e.storage == nil {
		e.storage = make(map[string]interface{})
	}
	e.storage[key] = value
	e.mu.Unlock()
}

// Get retrieves the value according to the key.
func (e *Message) Get(key string) (value interface{}, exists bool) {
	e.mu.RLock()
	value, exists = e.storage[key]
	e.mu.RUnlock()
	return
}

// MustGet retrieves the value according to the key.
// Panics if key does not exist.
func (e *Message) MustGet(key string) interface{} {
	if v, ok := e.Get(key); ok {
		return v
	}
	panic(fmt.Errorf("key `%s` does not exist", key))
}

// Remove deletes the key from storage.
func (e *Message) Remove(key string) {
	e.mu.Lock()
	delete(e.storage, key)
	e.mu.Unlock()
}