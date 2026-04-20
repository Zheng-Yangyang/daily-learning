package main

import (
	"context"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

func main() {
	// 1. 创建客户端
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://localhost:9200",
		},
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}

	// 2. 健康检查
	res, err := es.Info()
	if err != nil {
		log.Fatalf("连接失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("ES 返回错误: %s", res.String())
	}

	fmt.Println("✅ 连接成功！")

	// 3. 集群健康状态
	healthRes, err := es.Cluster.Health(
		es.Cluster.Health.WithContext(context.Background()),
		es.Cluster.Health.WithPretty(),
	)
	if err != nil {
		log.Fatalf("获取健康状态失败: %s", err)
	}
	defer healthRes.Body.Close()

	fmt.Println("\n📊 集群健康状态：")
	fmt.Println(healthRes.String())
}
