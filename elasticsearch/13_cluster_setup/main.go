package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
)

func main() {
	// 连接集群，配置三个节点地址
	// go-elasticsearch 会自动负载均衡
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://localhost:9200",
			"http://localhost:9201",
			"http://localhost:9202",
		},
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}
	ctx := context.Background()

	// 1. 集群基本信息
	clusterInfo(es, ctx)

	// 2. 节点详情
	nodesInfo(es, ctx)

	// 3. 集群健康状态详解
	clusterHealth(es, ctx)
}

// ----------------------------------------
// 1. 集群基本信息
// ----------------------------------------
func clusterInfo(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Info()
	if err != nil || res.IsError() {
		log.Fatalf("获取集群信息失败: %s", err)
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	fmt.Println("【1】集群基本信息：")
	fmt.Printf("   集群名称: %s\n", result["cluster_name"])
	version := result["version"].(map[string]interface{})
	fmt.Printf("   ES 版本:  %s\n", version["number"])
	fmt.Println()
}

// ----------------------------------------
// 2. 节点详情
// ----------------------------------------
func nodesInfo(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Cat.Nodes(
		es.Cat.Nodes.WithContext(ctx),
		es.Cat.Nodes.WithH("name", "ip", "heap.percent", "ram.percent", "cpu", "master", "node.role"),
		es.Cat.Nodes.WithFormat("json"),
	)
	if err != nil || res.IsError() {
		log.Fatalf("获取节点信息失败: %s", res.String())
	}
	defer res.Body.Close()

	var nodes []map[string]interface{}
	json.NewDecoder(res.Body).Decode(&nodes)

	fmt.Println("【2】集群节点详情：")
	fmt.Printf("   %-6s %-14s %-8s %-8s %-6s %-8s\n",
		"名称", "IP", "堆内存%", "内存%", "CPU%", "角色")
	fmt.Println("   " + strings.Repeat("-", 55))

	for _, node := range nodes {
		master := "slave"
		if node["master"] == "*" {
			master = "MASTER"
		}
		fmt.Printf("   %-6s %-14s %-8s %-8s %-6s %-8s\n",
			node["name"],
			node["ip"],
			node["heap.percent"],
			node["ram.percent"],
			node["cpu"],
			master,
		)
	}
	fmt.Println()
}

// ----------------------------------------
// 3. 集群健康状态详解
// ----------------------------------------
func clusterHealth(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Cluster.Health(
		es.Cluster.Health.WithContext(ctx),
		es.Cluster.Health.WithPretty(),
	)
	if err != nil || res.IsError() {
		log.Fatalf("获取健康状态失败: %s", res.String())
	}
	defer res.Body.Close()

	var health map[string]interface{}
	json.NewDecoder(res.Body).Decode(&health)

	status := health["status"].(string)
	statusIcon := map[string]string{
		"green":  "🟢",
		"yellow": "🟡",
		"red":    "🔴",
	}[status]

	fmt.Println("【3】集群健康状态：")
	fmt.Printf("   状态:           %s %s\n", statusIcon, status)
	fmt.Printf("   节点总数:        %.0f\n", health["number_of_nodes"])
	fmt.Printf("   数据节点数:      %.0f\n", health["number_of_data_nodes"])
	fmt.Printf("   主分片数:        %.0f\n", health["active_primary_shards"])
	fmt.Printf("   副本分片数:      %.0f\n", health["active_shards"])
	fmt.Printf("   未分配分片:      %.0f\n", health["unassigned_shards"])
	fmt.Printf("   迁移中分片:      %.0f\n", health["relocating_shards"])
	fmt.Println()

	// 状态说明
	fmt.Println("   状态含义：")
	fmt.Println("   🟢 green  — 所有主分片和副本分片都正常")
	fmt.Println("   🟡 yellow — 主分片正常，部分副本分片未分配（单节点时常见）")
	fmt.Println("   🔴 red    — 部分主分片不可用，有数据丢失风险")
}
