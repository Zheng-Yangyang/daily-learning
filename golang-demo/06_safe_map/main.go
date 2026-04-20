package main

import (
	"fmt"
	"sync"
)

// SafeMap 并发安全的 map
type SafeMap struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewSafeMap() *SafeMap {
	return &SafeMap{
		data: make(map[string]string),
	}
}

func (m *SafeMap) Set(key, value string) {
	m.mu.Lock() // 写操作：独占锁
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *SafeMap) Get(key string) (string, bool) {
	m.mu.RLock() // 读操作：共享锁，允许多个 goroutine 同时读
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	return val, ok
}

func (m *SafeMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

func main() {
	sm := NewSafeMap()
	var wg sync.WaitGroup

	// 5 个写 goroutine
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			sm.Set(key, value)
			fmt.Printf("set %s = %s\n", key, value)
		}()
	}

	// 5 个读 goroutine
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("key%d", i)
			val, ok := sm.Get(key)
			if ok {
				fmt.Printf("get %s = %s\n", key, val)
			} else {
				fmt.Printf("get %s = not found\n", key)
			}
		}()
	}

	wg.Wait()

	// 删除验证
	sm.Delete("key1")
	_, ok := sm.Get("key1")
	fmt.Printf("after delete, key1 exists: %v\n", ok)
}
