package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	// ============================================================
	// 启动 RPC Server
	// ============================================================
	server := NewServer()
	server.Register(&UserService{})

	// 后台启动服务
	go func() {
		if err := server.Start(":9000"); err != nil {
			fmt.Printf("Server 启动失败: %v\n", err)
		}
	}()

	// 等服务启动
	time.Sleep(100 * time.Millisecond)

	// ============================================================
	// 启动 RPC Client
	// ============================================================
	client, err := NewClient(":9000")
	if err != nil {
		fmt.Printf("Client 启动失败: %v\n", err)
		return
	}
	defer client.Close()

	// ============================================================
	// 场景一：基本调用
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景一：基本 RPC 调用")
	fmt.Println("========================================")

	reply, err := client.Call("UserService.GetUser", map[string]interface{}{
		"id": 1,
	})
	if err != nil {
		fmt.Printf("[Client] 调用失败: %v\n", err)
	} else {
		fmt.Printf("[Client] 收到响应: %v\n\n", reply)
	}

	// 调用 CreateOrder
	reply, err = client.Call("UserService.CreateOrder", map[string]interface{}{
		"user_id": 1,
		"amount":  299.99,
	})
	if err != nil {
		fmt.Printf("[Client] 调用失败: %v\n", err)
	} else {
		fmt.Printf("[Client] 收到响应: %v\n", reply)
	}

	// ============================================================
	// 场景二：调用不存在的用户，演示错误处理
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景二：调用不存在的用户")
	fmt.Println("========================================")

	reply, err = client.Call("UserService.GetUser", map[string]interface{}{
		"id": 99,
	})
	if err != nil {
		fmt.Printf("[Client] 业务错误（正常）: %v\n", err)
	} else {
		fmt.Printf("[Client] 收到响应: %v\n", reply)
	}

	// ============================================================
	// 场景三：并发调用，演示 seq 匹配机制
	// ============================================================
	fmt.Println("\n========================================")
	fmt.Println(" 场景三：3个并发调用，seq保证响应正确匹配")
	fmt.Println("========================================")

	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			reply, err := client.Call("UserService.GetUser", map[string]interface{}{
				"id": id,
			})
			if err != nil {
				fmt.Printf("[Client] goroutine-%d 失败: %v\n", id, err)
			} else {
				fmt.Printf("[Client] goroutine-%d 收到: %v\n", id, reply)
			}
		}(i)
	}
	wg.Wait()

	fmt.Println("\n========================================")
	fmt.Println(" 演示完毕")
	fmt.Println("========================================")
	fmt.Println(`
手写 RPC 和 Kitex 的对比：

手写版                    Kitex 工业级
──────────────────────    ──────────────────────
JSON 编解码               Thrift/Protobuf 二进制（快10倍+）
单连接                    连接池（并发性能）
无服务发现                对接 ETCD/Nacos
无超时控制                完整的超时/重试机制
无熔断限流                内置熔断器+限流器
手动反射调用              代码生成（kitex 工具自动生成 stub）
`)
}
