package main

import (
	"fmt"
	"log"
	"net"
	"order-service/handler"
	order "order-service/kitex_gen/order/orderservice"

	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	fmt.Println("[OrderService] 启动中...")

	svr := order.NewServer(
		&handler.OrderServiceImpl{},
		server.WithServiceAddr(&net.TCPAddr{Port: 8002}),
		server.WithLimit(&limit.Option{
			MaxConnections: 100,
			MaxQPS:         1000,
		}),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "order-service",
		}),
	)

	if err := svr.Run(); err != nil {
		log.Fatal(err)
	}
}
