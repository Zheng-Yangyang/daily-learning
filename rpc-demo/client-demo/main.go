package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/retry"
	"github.com/cloudwego/kitex/pkg/rpcinfo"

	userkit    "client-demo/kitex_gen_user/user"
	userclient "client-demo/kitex_gen_user/user/userservice"
)

func call(cli userclient.Client, ctx context.Context, id int64, label string) {
	resp, err := cli.GetUser(ctx, &userkit.GetUserRequest{UserId: id})
	if err != nil {
		fmt.Printf("  [%s] user_id=%d ✗ %v\n", label, id, err)
	} else {
		fmt.Printf("  [%s] user_id=%d ✓ %s\n", label, id, resp.Name)
	}
}

func triggerFail() {
	resp, err := http.Get("http://localhost:8011/fail")
	if err == nil {
		resp.Body.Close()
	}
}

func triggerRecover() {
	resp, err := http.Get("http://localhost:8011/recover")
	if err == nil {
		resp.Body.Close()
	}
}

func main() {
	genKey := func(ri rpcinfo.RPCInfo) string {
		return ri.To().ServiceName() + "." + ri.To().Method()
	}

	cbSuite := circuitbreak.NewCBSuite(genKey)
	cbSuite.UpdateServiceCBConfig("user-service.GetUser", circuitbreak.CBConfig{
		Enable:    true,
		ErrRate:   0.5,
		MinSample: 5,
	})

	cli, err := userclient.NewClient(
		"user-service",
		client.WithHostPorts("127.0.0.1:8001"),
		client.WithCircuitBreaker(cbSuite),
		client.WithFailureRetry(retry.NewFailurePolicy()),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println(" 场景一：正常状态，5个请求")
	fmt.Println("========================================")
	for i := 1; i <= 5; i++ {
		call(cli, ctx, 1, fmt.Sprintf("正常 req-%02d", i))
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n========================================")
	fmt.Println(" 场景二：故障触发，观察熔断器打开")
	fmt.Println("========================================")
	triggerFail()
	time.Sleep(200 * time.Millisecond)
	for i := 1; i <= 10; i++ {
		call(cli, ctx, 1, fmt.Sprintf("故障 req-%02d", i))
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n========================================")
	fmt.Println(" 场景三：恢复服务，等熔断器关闭")
	fmt.Println("========================================")
	triggerRecover()
	fmt.Println("  等待10秒...")
	time.Sleep(10 * time.Second)

	for i := 1; i <= 5; i++ {
		call(cli, ctx, 1, fmt.Sprintf("恢复 req-%02d", i))
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println("\n完成。")
}
