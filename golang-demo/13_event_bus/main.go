package main

import (
	"fmt"
	"sync"
)

type Handler func(data any)

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]Handler
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]Handler),
	}
}

// Subscribe 订阅某个 topic
func (eb *EventBus) Subscribe(topic string, handler Handler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.subscribers[topic] = append(eb.subscribers[topic], handler)
}

// Publish 发布事件，异步通知所有订阅者
func (eb *EventBus) Publish(topic string, data any) {
	eb.mu.RLock()
	handlers := make([]Handler, len(eb.subscribers[topic]))
	copy(handlers, eb.subscribers[topic]) // 复制一份，避免长时间持锁
	eb.mu.RUnlock()

	var wg sync.WaitGroup
	for _, handler := range handlers {
		wg.Add(1)
		handler := handler
		go func() {
			defer wg.Done()
			handler(data)
		}()
	}
	wg.Wait()
}

// Unsubscribe 取消订阅
func (eb *EventBus) Unsubscribe(topic string, handler Handler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	handlers := eb.subscribers[topic]
	for i, h := range handlers {
		// 通过函数指针比较找到对应的 handler
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			eb.subscribers[topic] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

func main() {
	eb := NewEventBus()

	// 订阅 order.created 事件，两个订阅者
	eb.Subscribe("order.created", func(data any) {
		fmt.Printf("subscriber1 received: topic=order.created data=%v\n", data)
	})
	eb.Subscribe("order.created", func(data any) {
		fmt.Printf("subscriber2 received: topic=order.created data=%v\n", data)
	})

	// 订阅 user.login 事件，一个订阅者
	eb.Subscribe("user.login", func(data any) {
		fmt.Printf("subscriber3 received: topic=user.login   data=%v\n", data)
	})

	var wg sync.WaitGroup

	// 并发发布事件
	wg.Add(2)
	go func() {
		defer wg.Done()
		eb.Publish("order.created", "order_001")
	}()
	go func() {
		defer wg.Done()
		eb.Publish("user.login", "user_888")
	}()

	wg.Wait()
	fmt.Println("all done")
}
