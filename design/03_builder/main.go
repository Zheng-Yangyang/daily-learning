package main

import (
	"fmt"
	"strings"
	"time"
)

// ========================================
// 建造者模式 Builder Pattern
//
// 场景：构建 HTTP 请求客户端配置
//
// 解决的问题：
//   当一个对象有很多可选参数时，构造函数会变成这样：
//   NewClient("url", 30, true, false, 3, nil, nil, "v2", ...)
//   根本不知道每个参数是什么意思，且大部分用默认值就行
//
// Builder 让你这样写：
//   client := NewClientBuilder("http://api.example.com").
//       Timeout(30 * time.Second).
//       Retry(3).
//       WithAuth("token-xxx").
//       Build()
// ========================================

// ----------------------------------------
// 产品：HTTPClient（字段很多，大部分可选）
// ----------------------------------------

type HTTPClient struct {
	baseURL    string
	timeout    time.Duration
	maxRetries int
	authToken  string
	headers    map[string]string
	debug      bool
	apiVersion string
}

func (c *HTTPClient) Get(path string) string {
	headers := []string{}
	for k, v := range c.headers {
		headers = append(headers, fmt.Sprintf("%s:%s", k, v))
	}
	return fmt.Sprintf(
		"GET %s%s\n     timeout=%v retries=%d auth=%v debug=%v headers=[%s]",
		c.baseURL, path,
		c.timeout, c.maxRetries,
		c.authToken != "", c.debug,
		strings.Join(headers, ", "),
	)
}

// ----------------------------------------
// Builder：链式调用，每个方法返回 *Builder
// ----------------------------------------

type ClientBuilder struct {
	client *HTTPClient
	errors []string // 收集校验错误，Build() 时统一返回
}

func NewClientBuilder(baseURL string) *ClientBuilder {
	return &ClientBuilder{
		client: &HTTPClient{
			baseURL:    baseURL,
			timeout:    10 * time.Second, // 默认值
			maxRetries: 1,
			apiVersion: "v1",
			headers:    make(map[string]string),
		},
	}
}

func (b *ClientBuilder) Timeout(d time.Duration) *ClientBuilder {
	if d <= 0 {
		b.errors = append(b.errors, "timeout 必须大于 0")
		return b
	}
	b.client.timeout = d
	return b
}

func (b *ClientBuilder) Retry(n int) *ClientBuilder {
	if n < 0 {
		b.errors = append(b.errors, "retry 不能为负数")
		return b
	}
	b.client.maxRetries = n
	return b
}

func (b *ClientBuilder) WithAuth(token string) *ClientBuilder {
	b.client.authToken = token
	b.client.headers["Authorization"] = "Bearer " + token
	return b
}

func (b *ClientBuilder) Header(key, value string) *ClientBuilder {
	b.client.headers[key] = value
	return b
}

func (b *ClientBuilder) Debug() *ClientBuilder {
	b.client.debug = true
	return b
}

func (b *ClientBuilder) APIVersion(v string) *ClientBuilder {
	b.client.apiVersion = v
	b.client.headers["X-API-Version"] = v
	return b
}

// Build：校验 + 返回最终对象
func (b *ClientBuilder) Build() (*HTTPClient, error) {
	if b.client.baseURL == "" {
		b.errors = append(b.errors, "baseURL 不能为空")
	}
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("构建失败：%s", strings.Join(b.errors, "; "))
	}
	return b.client, nil
}

// ----------------------------------------
// Go 更地道的写法：Functional Options 模式
// 和 Builder 解决同一问题，Go 标准库更常用这种
// ----------------------------------------

type ServerConfig struct {
	host        string
	port        int
	maxConn     int
	readTimeout time.Duration
	tlsEnabled  bool
}

// Option 是一个函数类型
type Option func(*ServerConfig)

// 每个配置项是一个返回 Option 的函数
func WithPort(port int) Option {
	return func(c *ServerConfig) {
		c.port = port
	}
}

func WithMaxConn(n int) Option {
	return func(c *ServerConfig) {
		c.maxConn = n
	}
}

func WithReadTimeout(d time.Duration) Option {
	return func(c *ServerConfig) {
		c.readTimeout = d
	}
}

func WithTLS() Option {
	return func(c *ServerConfig) {
		c.tlsEnabled = true
	}
}

// NewServer：默认值 + 应用所有 options
func NewServer(host string, opts ...Option) *ServerConfig {
	// 先设默认值
	cfg := &ServerConfig{
		host:        host,
		port:        8080,
		maxConn:     100,
		readTimeout: 30 * time.Second,
	}
	// 依次应用调用方传入的选项
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (s *ServerConfig) String() string {
	scheme := "http"
	if s.tlsEnabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d (maxConn=%d timeout=%v)",
		scheme, s.host, s.port, s.maxConn, s.readTimeout)
}

// ========================================
// main
// ========================================

func main() {
	fmt.Println("=== Builder 模式：链式调用构建 HTTPClient ===")

	// 场景一：完整配置
	client, err := NewClientBuilder("https://api.example.com").
		Timeout(30*time.Second).
		Retry(3).
		WithAuth("tok-abc123").
		Header("X-Request-ID", "req-001").
		APIVersion("v2").
		Debug().
		Build()

	if err != nil {
		fmt.Println("错误:", err)
	} else {
		fmt.Println("完整配置客户端:")
		fmt.Println(" ", client.Get("/users"))
	}

	// 场景二：最简配置（大部分用默认值）
	simple, _ := NewClientBuilder("https://api.example.com").Build()
	fmt.Println("\n最简配置客户端:")
	fmt.Println(" ", simple.Get("/health"))

	// 场景三：触发校验错误
	fmt.Println("\n=== 触发校验错误 ===")
	_, err = NewClientBuilder("https://api.example.com").
		Timeout(-1 * time.Second). // 非法值
		Retry(-5).                 // 非法值
		Build()
	fmt.Println("错误:", err)

	// ----------------------------------------
	fmt.Println("\n=== Functional Options：Go 更地道的写法 ===")

	// 只传需要的参数，其余用默认值
	s1 := NewServer("localhost")
	fmt.Println("默认配置:", s1)

	s2 := NewServer("0.0.0.0",
		WithPort(9090),
		WithMaxConn(500),
		WithTLS(),
		WithReadTimeout(60*time.Second),
	)
	fmt.Println("自定义配置:", s2)

	// 只改一个参数，其他保持默认
	s3 := NewServer("api.internal", WithPort(8443), WithTLS())
	fmt.Println("部分配置:", s3)
}
