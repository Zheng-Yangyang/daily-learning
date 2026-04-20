package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
)

// ============================================================
// RPC Server
// 核心职责：
// 1. 注册服务（把结构体的方法暴露出去）
// 2. 监听网络连接
// 3. 收到请求 → 找到对应方法 → 反射调用 → 返回结果
// ============================================================

type Server struct {
	services map[string]*serviceInfo // 注册的服务，key = "ServiceName.MethodName"
	codec    *Codec
}

// serviceInfo 存储一个注册的服务
type serviceInfo struct {
	name    string
	value   reflect.Value // 服务实例
	methods map[string]reflect.Method
}

func NewServer() *Server {
	return &Server{
		services: make(map[string]*serviceInfo),
		codec:    &Codec{},
	}
}

// Register 注册一个服务
// 传入服务实例，自动提取所有导出方法
func (s *Server) Register(service interface{}) error {
	t := reflect.TypeOf(service)
	v := reflect.ValueOf(service)

	// 服务名 = 结构体类型名，如 "UserService"
	serviceName := t.Elem().Name()
	info := &serviceInfo{
		name:    serviceName,
		value:   v,
		methods: make(map[string]reflect.Method),
	}

	// 遍历所有导出方法，存起来
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		info.methods[method.Name] = method
		fmt.Printf("[Server] 注册方法: %s.%s\n", serviceName, method.Name)
	}

	s.services[serviceName] = info
	return nil
}

// Start 启动服务，监听端口
func (s *Server) Start(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听失败: %v", err)
	}
	fmt.Printf("[Server] 启动成功，监听 %s\n", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[Server] 接受连接失败: %v\n", err)
			continue
		}
		// 每个连接起一个 goroutine 处理，这是 Go 网络编程的标准模式
		go s.handleConn(conn)
	}
}

// handleConn 处理一个客户端连接
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		// 读取一行（客户端每个请求以 \n 结尾）
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return // 连接断开
		}

		// 反序列化请求
		var req Request
		if err := s.codec.Decode(line, &req); err != nil {
			fmt.Printf("[Server] 解码请求失败: %v\n", err)
			continue
		}

		fmt.Printf("[Server] 收到请求: %s (seq=%d) args=%v\n",
			req.ServiceMethod, req.Seq, req.Args)

		// 处理请求，拿到响应
		resp := s.handleRequest(&req)

		// 序列化响应，发回客户端
		data, _ := s.codec.Encode(resp)
		data = append(data, '\n') // 加换行符作为消息分隔符
		conn.Write(data)
	}
}

// handleRequest 找到对应方法，用反射调用它
func (s *Server) handleRequest(req *Request) *Response {
	resp := &Response{
		ServiceMethod: req.ServiceMethod,
		Seq:           req.Seq,
	}

	// 解析 "ServiceName.MethodName"
	var serviceName, methodName string
	fmt.Sscanf(req.ServiceMethod, "%s", &req.ServiceMethod)
	for i, c := range req.ServiceMethod {
		if c == '.' {
			serviceName = req.ServiceMethod[:i]
			methodName = req.ServiceMethod[i+1:]
			break
		}
	}

	// 找服务
	svc, ok := s.services[serviceName]
	if !ok {
		resp.Error = fmt.Sprintf("服务不存在: %s", serviceName)
		return resp
	}

	// 找方法
	method, ok := svc.methods[methodName]
	if !ok {
		resp.Error = fmt.Sprintf("方法不存在: %s", methodName)
		return resp
	}

	// 构造参数：把 JSON args 转成方法需要的类型
	// 方法签名约定：func (s *XxxService) Method(args map[string]interface{}) (interface{}, error)
	argsValue := reflect.ValueOf(req.Args)

	// 反射调用
	results := method.Func.Call([]reflect.Value{svc.value, argsValue})

	// 处理返回值（约定返回 interface{}, error）
	if len(results) == 2 {
		if !results[1].IsNil() {
			resp.Error = results[1].Interface().(error).Error()
		} else {
			resp.Reply = results[0].Interface()
		}
	}

	return resp
}

// ============================================================
// 业务服务：UserService
// 这就是业务开发者写的代码，完全不用关心网络和序列化
// ============================================================

type UserService struct{}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// GetUser 根据 ID 查询用户
func (u *UserService) GetUser(args interface{}) (interface{}, error) {
	// 从 args 里拿参数
	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("参数格式错误")
	}

	id := int(argsMap["id"].(float64)) // JSON 数字默认是 float64

	// 模拟数据库查询
	users := map[int]User{
		1: {ID: 1, Name: "张三", Age: 25},
		2: {ID: 2, Name: "李四", Age: 30},
		3: {ID: 3, Name: "王五", Age: 28},
	}

	user, exists := users[id]
	if !exists {
		return nil, fmt.Errorf("用户不存在: id=%d", id)
	}

	fmt.Printf("[UserService] 查询用户 id=%d，返回: %+v\n", id, user)
	return user, nil
}

// CreateOrder 创建订单
func (u *UserService) CreateOrder(args interface{}) (interface{}, error) {
	argsMap := args.(map[string]interface{})
	userID := int(argsMap["user_id"].(float64))
	amount := argsMap["amount"].(float64)

	orderNo := fmt.Sprintf("ORD-%d-%d", userID, 1000+userID)
	fmt.Printf("[UserService] 创建订单 user_id=%d amount=%.2f，订单号: %s\n",
		userID, amount, orderNo)

	return map[string]interface{}{
		"order_no": orderNo,
		"status":   "created",
	}, nil
}

// 把 handleRequest 里解析 ServiceMethod 的逻辑抽出来，避免用 Sscanf
func parseServiceMethod(sm string) (string, string) {
	for i, c := range sm {
		if c == '.' {
			return sm[:i], sm[i+1:]
		}
	}
	return sm, ""
}

// 修正 handleRequest，用上面的函数
func init() {
	// 用 json 处理嵌套结构
	_ = json.Marshal
}
