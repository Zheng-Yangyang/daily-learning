package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

const indexName = "products_cluster"

func newClient() *elasticsearch.Client {
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
	return es
}

func main() {
	es := newClient()
	ctx := context.Background()

	// 1. 记录故障前状态
	fmt.Println("========== 故障前状态 ==========")
	showClusterStatus(es, ctx)
	showShards(es, ctx)

	// 2. 提示手动 kill 节点
	fmt.Println("========== 模拟故障 ==========")
	fmt.Println("请在另一个终端执行以下命令，停掉 es03 节点：")
	fmt.Println()
	fmt.Println("   docker stop es03")
	fmt.Println()
	fmt.Println("执行完毕后按回车继续...")
	fmt.Scanln()

	// 3. 故障后立即检查
	fmt.Println("========== 故障后状态 ==========")
	showClusterStatus(es, ctx)
	showShards(es, ctx)

	// 4. 验证集群仍然可以读写
	verifyReadWrite(es, ctx)

	// 5. 等待集群自愈
	fmt.Println("========== 等待集群自愈 ==========")
	waitForRecovery(es, ctx)

	// 6. 提示恢复节点
	fmt.Println("请在另一个终端执行以下命令，恢复 es03 节点：")
	fmt.Println()
	fmt.Println("   docker start es03")
	fmt.Println()
	fmt.Println("执行完毕后按回车继续...")
	fmt.Scanln()

	// 7. 节点恢复后状态
	fmt.Println("========== 节点恢复后 ==========")
	time.Sleep(5 * time.Second) // 等待节点加入集群
	showClusterStatus(es, ctx)
	showShards(es, ctx)
}

// ----------------------------------------
// 集群状态
// ----------------------------------------
func showClusterStatus(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Cluster.Health(
		es.Cluster.Health.WithContext(ctx),
	)
	if err != nil || res.IsError() {
		fmt.Println("   ❌ 无法获取集群状态")
		return
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

	fmt.Printf("集群状态: %s %s | 节点数: %.0f | 主分片: %.0f | 未分配: %.0f\n",
		statusIcon, status,
		health["number_of_nodes"],
		health["active_primary_shards"],
		health["unassigned_shards"],
	)
	fmt.Println()
}

// ----------------------------------------
// 分片分布
// ----------------------------------------
func showShards(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Cat.Shards(
		es.Cat.Shards.WithContext(ctx),
		es.Cat.Shards.WithIndex(indexName),
		es.Cat.Shards.WithFormat("json"),
		es.Cat.Shards.WithH("shard", "prirep", "state", "docs", "node"),
	)
	if err != nil || res.IsError() {
		fmt.Println("   ❌ 无法获取分片信息")
		return
	}
	defer res.Body.Close()

	var shards []map[string]interface{}
	json.NewDecoder(res.Body).Decode(&shards)

	fmt.Printf("   %-6s %-8s %-12s %-6s %-8s\n", "分片", "类型", "状态", "文档数", "节点")
	fmt.Println("   " + strings.Repeat("-", 45))
	for _, shard := range shards {
		shardType := "副本"
		if shard["prirep"] == "p" {
			shardType = "主分片"
		}
		node := shard["node"]
		if node == nil {
			node = "❌ 无节点"
		}
		fmt.Printf("   %-6s %-8s %-12s %-6s %-8s\n",
			shard["shard"], shardType, shard["state"], shard["docs"], node)
	}
	fmt.Println()
}

// ----------------------------------------
// 验证故障后仍可读写
// ----------------------------------------
func verifyReadWrite(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("========== 验证故障后读写 ==========")

	// 写入一条新数据
	body := `{"name":"故障测试商品","brand":"Test","price":999}`
	res, err := es.Index(
		indexName,
		strings.NewReader(body),
		es.Index.WithContext(ctx),
		es.Index.WithRefresh("true"),
	)
	if err != nil || res.IsError() {
		fmt.Println("   ❌ 写入失败！集群不可用")
		return
	}
	res.Body.Close()
	fmt.Println("   ✅ 写入成功！节点故障不影响写入")

	// 查询数据
	query := `{"query":{"match_all":{}},"size":1}`
	res2, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res2.IsError() {
		fmt.Println("   ❌ 查询失败！集群不可用")
		return
	}
	defer res2.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res2.Body).Decode(&result)
	total := result["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)
	fmt.Printf("   ✅ 查询成功！当前共 %.0f 条数据\n\n", total)
}

// ----------------------------------------
// 等待集群自愈（yellow → green）
// ----------------------------------------
func waitForRecovery(es *elasticsearch.Client, ctx context.Context) {
	fmt.Print("   等待集群自愈中")
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		fmt.Print(".")

		res, err := es.Cluster.Health(es.Cluster.Health.WithContext(ctx))
		if err != nil || res.IsError() {
			res.Body.Close()
			continue
		}

		var health map[string]interface{}
		json.NewDecoder(res.Body).Decode(&health)
		res.Body.Close()

		status := health["status"].(string)
		unassigned := health["unassigned_shards"].(float64)

		if status == "yellow" && unassigned == 0 {
			fmt.Printf("\n   🟡 集群已自愈为 yellow（主分片全部正常，副本待恢复）\n\n")
			return
		}
	}
	fmt.Println("\n   ⚠️  自愈超时，请检查集群状态")
	fmt.Println()
}
