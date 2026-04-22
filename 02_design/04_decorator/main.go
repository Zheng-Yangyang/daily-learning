package main

import (
	"fmt"
	"strings"
	"time"
)

// ========================================
// 装饰器模式 Decorator Pattern
//
// 场景：HTTP 中间件系统
//
// 解决的问题：
//   给一个对象动态添加功能，不修改原始代码
//   比如：给 HTTP Handler 加上 日志、限流、鉴权、耗时统计
//   这些功能可以自由组合，像套娃一样一层一层包
//
// 核心结构：
//   装饰器和被装饰者实现同一个接口
//   装饰器内部持有被装饰者的引用
//   调用时先做自己的逻辑，再调用被装饰者
// ========================================

// ----------------------------------------
// 核心接口：Handler
// ----------------------------------------

type Handler interface {
	Handle(req *Request) *Response
}

type Request struct {
	Method  string
	Path    string
	UserID  string
	Headers map[string]string
}

type Response struct {
	Status  int
	Body    string
	Headers map[string]string
}

func (r *Response) String() string {
	return fmt.Sprintf("HTTP %d | %s", r.Status, r.Body)
}

// ----------------------------------------
// 真实 Handler：核心业务逻辑
// ----------------------------------------

type UserHandler struct{}

func (h *UserHandler) Handle(req *Request) *Response {
	return &Response{
		Status:  200,
		Body:    fmt.Sprintf("用户数据: {id: %s, name: 'Alice'}", req.UserID),
		Headers: map[string]string{"Content-Type": "application/json"},
	}
}

// ----------------------------------------
// 装饰器一：日志中间件
// ----------------------------------------

type LoggingDecorator struct {
	next Handler // 持有下一层的引用
}

func WithLogging(next Handler) Handler {
	return &LoggingDecorator{next: next}
}

func (d *LoggingDecorator) Handle(req *Request) *Response {
	fmt.Printf("  [Log] → %s %s userID=%s\n", req.Method, req.Path, req.UserID)
	start := time.Now()

	resp := d.next.Handle(req) // 调用下一层

	fmt.Printf("  [Log] ← %d 耗时=%v\n", resp.Status, time.Since(start))
	return resp
}

// ----------------------------------------
// 装饰器二：鉴权中间件
// ----------------------------------------

type AuthDecorator struct {
	next       Handler
	validToken string
}

func WithAuth(token string, next Handler) Handler {
	return &AuthDecorator{next: next, validToken: token}
}

func (d *AuthDecorator) Handle(req *Request) *Response {
	token := req.Headers["Authorization"]
	fmt.Printf("  [Auth] 校验 token: %s\n", token)

	if token != "Bearer "+d.validToken {
		fmt.Println("  [Auth] ✗ 鉴权失败")
		return &Response{Status: 401, Body: "Unauthorized", Headers: map[string]string{}}
	}

	fmt.Println("  [Auth] ✓ 鉴权通过")
	return d.next.Handle(req)
}

// ----------------------------------------
// 装饰器三：限流中间件
// ----------------------------------------

type RateLimitDecorator struct {
	next        Handler
	maxRequests int
	count       int
}

func WithRateLimit(max int, next Handler) Handler {
	return &RateLimitDecorator{next: next, maxRequests: max}
}

func (d *RateLimitDecorator) Handle(req *Request) *Response {
	d.count++
	fmt.Printf("  [RateLimit] 当前请求数: %d/%d\n", d.count, d.maxRequests)

	if d.count > d.maxRequests {
		fmt.Println("  [RateLimit] ✗ 触发限流")
		return &Response{Status: 429, Body: "Too Many Requests", Headers: map[string]string{}}
	}

	fmt.Println("  [RateLimit] ✓ 通过")
	return d.next.Handle(req)
}

// ----------------------------------------
// 装饰器四：耗时统计中间件
// ----------------------------------------

type MetricsDecorator struct {
	next Handler
	name string
}

func WithMetrics(name string, next Handler) Handler {
	return &MetricsDecorator{next: next, name: name}
}

func (d *MetricsDecorator) Handle(req *Request) *Response {
	start := time.Now()
	resp := d.next.Handle(req)
	elapsed := time.Since(start)

	// 实际项目中这里会上报到 Prometheus / Datadog
	fmt.Printf("  [Metrics] %s 耗时=%v status=%d\n", d.name, elapsed, resp.Status)
	return resp
}

// ----------------------------------------
// 辅助函数：打印分隔线
// ----------------------------------------

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 50))
}

// ========================================
// main
// ========================================

func main() {
	req := &Request{
		Method:  "GET",
		Path:    "/api/users/123",
		UserID:  "123",
		Headers: map[string]string{"Authorization": "Bearer secret-token"},
	}

	section("=== 场景一：只加日志 ===")
	h1 := WithLogging(&UserHandler{})
	fmt.Println("结果:", h1.Handle(req))

	section("=== 场景二：日志 + 鉴权（正确 token）===")
	// 套娃：请求先进 Logging → 再进 Auth → 最后到 UserHandler
	h2 := WithLogging(
		WithAuth("secret-token",
			&UserHandler{},
		),
	)
	fmt.Println("结果:", h2.Handle(req))

	section("=== 场景三：日志 + 鉴权（错误 token）===")
	badReq := &Request{
		Method:  "GET",
		Path:    "/api/users/123",
		UserID:  "123",
		Headers: map[string]string{"Authorization": "Bearer wrong-token"},
	}
	fmt.Println("结果:", h2.Handle(badReq))

	section("=== 场景四：完整中间件链 日志+限流+鉴权+业务 ===")
	// 从外到内：Metrics → Logging → RateLimit → Auth → UserHandler
	h3 := WithMetrics("user-api",
		WithLogging(
			WithRateLimit(2,
				WithAuth("secret-token",
					&UserHandler{},
				),
			),
		),
	)

	for i := 1; i <= 3; i++ {
		fmt.Printf("\n--- 第 %d 次请求 ---\n", i)
		fmt.Println("结果:", h3.Handle(req))
	}

	section("=== 关键理解：装饰器调用链 ===")
	fmt.Println(`
  请求进入方向 →→→
  Metrics → Logging → RateLimit → Auth → UserHandler
                                               ↓ 处理
  响应返回方向 ←←←
  Metrics ← Logging ← RateLimit ← Auth ← UserHandler

  每一层只负责自己的职责，可以自由组合拆卸`)
}
