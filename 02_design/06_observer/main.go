package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ========================================
// 观察者模式 Observer Pattern
//
// 场景：电商订单系统
// 订单状态变更时，需要触发多个下游动作：
//   - 发送通知（邮件/SMS）
//   - 更新库存
//   - 记录日志
//   - 触发积分
//
// 解决的问题：
//   不用观察者时，OrderService 要直接调用
//   NotifyService、InventoryService、LogService...
//   每加一个下游就要改 OrderService，耦合极深
//
//   用观察者后：OrderService 只管发事件
//   谁关心谁自己来订阅，OrderService 完全不知道下游
// ========================================

// ----------------------------------------
// 事件定义
// ----------------------------------------

type OrderStatus string

const (
	StatusCreated   OrderStatus = "CREATED"
	StatusPaid      OrderStatus = "PAID"
	StatusShipped   OrderStatus = "SHIPPED"
	StatusCancelled OrderStatus = "CANCELLED"
)

type OrderEvent struct {
	OrderID   string
	UserID    string
	Amount    float64
	Status    OrderStatus
	Timestamp time.Time
}

func (e OrderEvent) String() string {
	return fmt.Sprintf("Order[%s] user=%s amount=%.2f status=%s",
		e.OrderID, e.UserID, e.Amount, e.Status)
}

// ----------------------------------------
// 观察者接口
// ----------------------------------------

type Observer interface {
	OnEvent(event OrderEvent)
	Name() string
}

// ----------------------------------------
// 被观察者（Subject）：OrderService
// ----------------------------------------

type OrderService struct {
	mu        sync.RWMutex
	observers map[OrderStatus][]Observer // 按事件类型订阅
	allObs    []Observer                 // 订阅所有事件
}

func NewOrderService() *OrderService {
	return &OrderService{
		observers: make(map[OrderStatus][]Observer),
	}
}

// Subscribe：订阅特定状态的事件
func (s *OrderService) Subscribe(status OrderStatus, obs Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers[status] = append(s.observers[status], obs)
	fmt.Printf("  [订阅] %s 订阅了 %s 事件\n", obs.Name(), status)
}

// SubscribeAll：订阅所有事件
func (s *OrderService) SubscribeAll(obs Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allObs = append(s.allObs, obs)
	fmt.Printf("  [订阅] %s 订阅了全部事件\n", obs.Name())
}

// Unsubscribe：取消订阅
func (s *OrderService) Unsubscribe(status OrderStatus, obs Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.observers[status]
	for i, o := range list {
		if o.Name() == obs.Name() {
			s.observers[status] = append(list[:i], list[i+1:]...)
			fmt.Printf("  [取消订阅] %s 取消了 %s 事件\n", obs.Name(), status)
			return
		}
	}
}

// notify：发布事件，通知所有订阅者（异步）
func (s *OrderService) notify(event OrderEvent) {
	s.mu.RLock()
	specific := append([]Observer{}, s.observers[event.Status]...)
	all := append([]Observer{}, s.allObs...)
	s.mu.RUnlock()

	observers := append(specific, all...)
	if len(observers) == 0 {
		return
	}

	fmt.Printf("\n  📢 发布事件: %s\n", event)

	var wg sync.WaitGroup
	for _, obs := range observers {
		wg.Add(1)
		go func(o Observer) { // 异步通知，互不阻塞
			defer wg.Done()
			o.OnEvent(event)
		}(obs)
	}
	wg.Wait()
}

// 业务方法：只管自己的逻辑，通过 notify 解耦下游
func (s *OrderService) CreateOrder(orderID, userID string, amount float64) {
	fmt.Printf("\n[OrderService] 创建订单 %s\n", orderID)
	// ... 写数据库等业务逻辑 ...
	s.notify(OrderEvent{
		OrderID: orderID, UserID: userID,
		Amount: amount, Status: StatusCreated,
		Timestamp: time.Now(),
	})
}

func (s *OrderService) PayOrder(orderID, userID string, amount float64) {
	fmt.Printf("\n[OrderService] 支付订单 %s\n", orderID)
	s.notify(OrderEvent{
		OrderID: orderID, UserID: userID,
		Amount: amount, Status: StatusPaid,
		Timestamp: time.Now(),
	})
}

func (s *OrderService) CancelOrder(orderID, userID string, amount float64) {
	fmt.Printf("\n[OrderService] 取消订单 %s\n", orderID)
	s.notify(OrderEvent{
		OrderID: orderID, UserID: userID,
		Amount: amount, Status: StatusCancelled,
		Timestamp: time.Now(),
	})
}

// ----------------------------------------
// 具体观察者们
// ----------------------------------------

// 通知服务
type NotificationObserver struct{}

func (o *NotificationObserver) Name() string { return "NotificationService" }
func (o *NotificationObserver) OnEvent(e OrderEvent) {
	switch e.Status {
	case StatusCreated:
		fmt.Printf("    [通知] 📧 邮件 → %s: 订单 %s 已创建，金额 ¥%.2f\n", e.UserID, e.OrderID, e.Amount)
	case StatusPaid:
		fmt.Printf("    [通知] 📱 SMS  → %s: 订单 %s 支付成功！\n", e.UserID, e.OrderID)
	case StatusCancelled:
		fmt.Printf("    [通知] 📧 邮件 → %s: 订单 %s 已取消，退款 ¥%.2f\n", e.UserID, e.OrderID, e.Amount)
	}
}

// 库存服务
type InventoryObserver struct{}

func (o *InventoryObserver) Name() string { return "InventoryService" }
func (o *InventoryObserver) OnEvent(e OrderEvent) {
	switch e.Status {
	case StatusCreated:
		fmt.Printf("    [库存] 锁定商品库存 orderID=%s\n", e.OrderID)
	case StatusCancelled:
		fmt.Printf("    [库存] 释放商品库存 orderID=%s\n", e.OrderID)
	}
}

// 积分服务
type PointsObserver struct{}

func (o *PointsObserver) Name() string { return "PointsService" }
func (o *PointsObserver) OnEvent(e OrderEvent) {
	if e.Status == StatusPaid {
		points := int(e.Amount / 10)
		fmt.Printf("    [积分] +%d 积分 → 用户 %s（消费 ¥%.2f）\n", points, e.UserID, e.Amount)
	}
}

// 审计日志（订阅所有事件）
type AuditObserver struct{}

func (o *AuditObserver) Name() string { return "AuditLog" }
func (o *AuditObserver) OnEvent(e OrderEvent) {
	fmt.Printf("    [审计] [%s] %s\n", e.Timestamp.Format("15:04:05.000"), e)
}

// ----------------------------------------
// Go Channel 版本：更地道的 Go 写法
// ----------------------------------------

type EventBus struct {
	ch   chan OrderEvent
	subs []func(OrderEvent)
}

func NewEventBus(bufSize int) *EventBus {
	bus := &EventBus{
		ch: make(chan OrderEvent, bufSize),
	}
	go bus.dispatch() // 后台消费
	return bus
}

func (b *EventBus) Subscribe(fn func(OrderEvent)) {
	b.subs = append(b.subs, fn)
}

func (b *EventBus) Publish(e OrderEvent) {
	b.ch <- e
}

func (b *EventBus) dispatch() {
	for e := range b.ch {
		for _, fn := range b.subs {
			fn(e)
		}
	}
}

// ========================================
// main
// ========================================

func main() {
	section("=== 观察者模式：订单事件系统 ===")

	svc := NewOrderService()

	// 各服务按需订阅自己关心的事件
	notify := &NotificationObserver{}
	inventory := &InventoryObserver{}
	points := &PointsObserver{}
	audit := &AuditObserver{}

	svc.Subscribe(StatusCreated, notify)
	svc.Subscribe(StatusCreated, inventory)
	svc.Subscribe(StatusPaid, notify)
	svc.Subscribe(StatusPaid, points)
	svc.Subscribe(StatusCancelled, notify)
	svc.Subscribe(StatusCancelled, inventory)
	svc.SubscribeAll(audit) // 审计订阅所有事件

	fmt.Println()
	svc.CreateOrder("ORD-001", "user-123", 299.00)
	time.Sleep(10 * time.Millisecond)

	svc.PayOrder("ORD-001", "user-123", 299.00)
	time.Sleep(10 * time.Millisecond)

	section("\n=== 取消订阅演示 ===")
	svc.Unsubscribe(StatusCancelled, notify)
	svc.CancelOrder("ORD-002", "user-456", 199.00)
	time.Sleep(10 * time.Millisecond)

	section("\n=== Channel 版本（更地道的 Go 写法）===")

	bus := NewEventBus(10)

	bus.Subscribe(func(e OrderEvent) {
		fmt.Printf("  [消费者1] 收到事件: %s\n", e)
	})
	bus.Subscribe(func(e OrderEvent) {
		if e.Status == StatusPaid {
			fmt.Printf("  [消费者2] 支付事件专项处理: orderID=%s\n", e.OrderID)
		}
	})

	bus.Publish(OrderEvent{OrderID: "ORD-003", UserID: "user-789", Amount: 99.0, Status: StatusCreated, Timestamp: time.Now()})
	bus.Publish(OrderEvent{OrderID: "ORD-003", UserID: "user-789", Amount: 99.0, Status: StatusPaid, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	section("\n=== 核心价值 ===")
	fmt.Println(`
  OrderService 不知道也不关心有多少个下游
  新增一个风控服务？直接 Subscribe，OrderService 零改动
  下游服务挂了？不影响 OrderService 和其他观察者
  这就是"开闭原则"在事件系统里的体现`)
}

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 50))
}
