package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// 熔断器三个状态
type State int

const (
	StateClosed   State = iota // 正常，请求放行
	StateOpen                  // 熔断，请求直接拒绝
	StateHalfOpen              // 试探，放一个请求过去
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED  ✓"
	case StateOpen:
		return "OPEN    ✗"
	case StateHalfOpen:
		return "HALF-OPEN ?"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	name string

	// 阈值配置
	maxFailures  int           // 连续失败多少次后熔断
	resetTimeout time.Duration // OPEN 状态等多久后进入 HALF-OPEN

	// 运行时状态
	mu           sync.Mutex
	state        State
	failures     int       // 当前连续失败次数
	lastFailure  time.Time // 最后一次失败时间
	totalReqs    int       // 统计：总请求数
	rejectedReqs int       // 统计：被熔断拒绝的请求数
}

var ErrCircuitOpen = errors.New("熔断器已打开，请求被拒绝")

func NewCircuitBreaker(name string, maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// Execute 通过熔断器执行一个函数
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	cb.totalReqs++

	// 判断当前状态，决定是否放行
	switch cb.state {
	case StateOpen:
		// 检查是否到了该试探的时间
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			// 进入半开状态，放一个请求过去试探
			cb.state = StateHalfOpen
			fmt.Printf("  [熔断器-%s] 状态变更: OPEN → %s，开始试探\n", cb.name, cb.state)
		} else {
			// 还没到时间，继续拒绝
			cb.rejectedReqs++
			cb.mu.Unlock()
			return ErrCircuitOpen
		}

	case StateHalfOpen:
		// 半开状态只放一个请求，其他的继续拒绝
		// 简化实现：半开时直接放行，由结果决定下一状态

	case StateClosed:
		// 正常状态，直接放行
	}

	cb.mu.Unlock()

	// 执行实际业务函数
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure(err)
	} else {
		cb.onSuccess()
	}

	return err
}

// onSuccess 请求成功时的状态转换
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateHalfOpen:
		// 试探成功，恢复正常
		cb.failures = 0
		cb.state = StateClosed
		fmt.Printf("  [熔断器-%s] 试探成功，状态变更: HALF-OPEN → CLOSED ✓\n", cb.name)
	case StateClosed:
		// 正常状态下成功，重置失败计数
		cb.failures = 0
	}
}

// onFailure 请求失败时的状态转换
func (cb *CircuitBreaker) onFailure(err error) {
	cb.lastFailure = time.Now()

	switch cb.state {
	case StateHalfOpen:
		// 试探失败，重新熔断
		cb.state = StateOpen
		fmt.Printf("  [熔断器-%s] 试探失败，状态变更: HALF-OPEN → OPEN ✗\n", cb.name)

	case StateClosed:
		cb.failures++
		if cb.failures >= cb.maxFailures {
			// 连续失败次数达到阈值，触发熔断
			cb.state = StateOpen
			fmt.Printf("  [熔断器-%s] 连续失败%d次，状态变更: CLOSED → OPEN ✗\n",
				cb.name, cb.failures)
		}
	}
}

// Stats 打印统计信息
func (cb *CircuitBreaker) Stats() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	fmt.Printf("  [统计] 总请求:%d  被拒绝:%d  当前状态:%s\n",
		cb.totalReqs, cb.rejectedReqs, cb.state)
}
