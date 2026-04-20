package main

import (
	"fmt"
	"log"
	"payment-system/api"
	"payment-system/config"
	"payment-system/dal"
	"payment-system/middleware"
	"payment-system/service"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 初始化 MySQL
	db, err := dal.NewDB(cfg.MySQL.DSN)
	if err != nil {
		log.Fatalf("MySQL 初始化失败: %v", err)
	}

	// 初始化 Redis
	redis, err := dal.NewRedis(cfg.Redis.Addr)
	if err != nil {
		log.Fatalf("Redis 初始化失败: %v", err)
	}

	// 初始化 Kafka（如果没启动 Kafka 也能运行，只是发消息会失败）
	mq := dal.NewMQ(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer mq.Close()

	// 初始化 Service 层
	paymentSvc := service.NewPaymentService(db, redis, mq)

	// 初始化 HTTP 路由
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.LoggerMiddleware())
	r.Use(middleware.RateLimitMiddleware())
	r.Use(gin.Recovery())

	// 注册路由
	h := api.NewHandler(paymentSvc)
	r.POST("/orders", h.CreateOrder)
	r.POST("/pay", h.Pay)
	r.GET("/users/:id", h.GetUser)

	// 健康检查
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	addr := fmt.Sprintf(":%d", cfg.HTTP.Port)
	fmt.Printf("[Server] 启动成功，监听 %s\n", addr)
	fmt.Println("[Server] 接口列表:")
	fmt.Println("  POST /orders  - 创建订单")
	fmt.Println("  POST /pay     - 支付")
	fmt.Println("  GET  /ping    - 健康检查")

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
