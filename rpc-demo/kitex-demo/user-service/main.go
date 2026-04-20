package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"user-service/handler"
	user "user-service/kitex_gen/user/userservice"

	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
)

func main() {
	fmt.Println("[UserService] 启动中...")

	// HTTP 控制接口，方便我们手动触发故障
	go func() {
		http.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
			handler.SetFailMode(true)
			w.Write([]byte("故障模式已开启\n"))
		})
		http.HandleFunc("/recover", func(w http.ResponseWriter, r *http.Request) {
			handler.SetFailMode(false)
			w.Write([]byte("已恢复正常\n"))
		})
		http.ListenAndServe(":8011", nil)
	}()

	svr := user.NewServer(
		&handler.UserServiceImpl{},
		server.WithServiceAddr(&net.TCPAddr{Port: 8001}),
		server.WithLimit(&limit.Option{
			MaxConnections: 100,
			MaxQPS:         1000,
		}),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "user-service",
		}),
	)

	if err := svr.Run(); err != nil {
		log.Fatal(err)
	}
}
