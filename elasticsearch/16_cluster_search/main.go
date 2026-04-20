package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
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

	// 1. 分布式搜索两阶段原理
	explainTwoPhaseSearch(es, ctx)

	// 2. 验证路由计算
	verifyRouting(es, ctx)

	// 3. 搜索偏好（preference）
	searchPreference(es, ctx)

	// 4. 负载均衡验证
	loadBalanceDemo(es, ctx)
}

// ----------------------------------------
// 1. 两阶段搜索原理说明 + explain API
// ----------------------------------------
func explainTwoPhaseSearch(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【1】分布式搜索两阶段原理：")
	fmt.Println()
	fmt.Println("   Query 阶段（Scatter）：")
	fmt.Println("   协调节点 → 广播请求到所有分片（主或副本）")
	fmt.Println("   每个分片本地搜索 → 返回 [文档ID + 评分] 给协调节点")
	fmt.Println("   协调节点合并所有分片结果 → 全局排序 → 取 Top N 个文档ID")
	fmt.Println()
	fmt.Println("   Fetch 阶段（Gather）：")
	fmt.Println("   协调节点 → 根据文档ID去对应分片拉取完整文档内容")
	fmt.Println("   汇总返回给客户端")
	fmt.Println()

	// 用 explain API 查看单条文档的评分详情和所在分片
	query := `{"query":{"match_all":{}},"size":1}`
	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(strings.NewReader(query)),
		es.Search.WithExplain(true), // 开启 explain，返回评分详情
	)
	if err != nil || res.IsError() {
		log.Fatalf("搜索失败: %s", res.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	hits := result["hits"].(map[string]interface{})["hits"].([]interface{})
	if len(hits) > 0 {
		h := hits[0].(map[string]interface{})
		fmt.Printf("   示例文档:\n")
		fmt.Printf("   _id:    %s\n", h["_id"])
		fmt.Printf("   _index: %s\n", h["_index"])
		fmt.Printf("   _score: %v\n", h["_score"])
		if explanation, ok := h["_explanation"]; ok {
			exp := explanation.(map[string]interface{})
			fmt.Printf("   评分说明: %s\n", exp["description"])
		}
	}
	fmt.Println()
}

// ----------------------------------------
// 2. 验证路由计算：同一个 ID 总是去同一个分片
// ----------------------------------------
func verifyRouting(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【2】路由验证：hash(id) % 3 决定文档在哪个分片：")
	fmt.Println()

	// 查询已有文档，看它们在哪个分片
	res, err := es.Cat.Shards(
		es.Cat.Shards.WithContext(ctx),
		es.Cat.Shards.WithIndex(indexName),
		es.Cat.Shards.WithFormat("json"),
		es.Cat.Shards.WithH("shard", "prirep", "docs", "node"),
	)
	if err != nil || res.IsError() {
		log.Fatalf("获取分片信息失败: %s", res.String())
	}
	defer res.Body.Close()

	var shards []map[string]interface{}
	json.NewDecoder(res.Body).Decode(&shards)

	fmt.Printf("   %-8s %-8s %-6s %-8s\n", "分片", "类型", "文档数", "节点")
	fmt.Println("   " + strings.Repeat("-", 35))
	for _, shard := range shards {
		if shard["prirep"] == "p" { // 只显示主分片
			fmt.Printf("   %-8s %-8s %-6s %-8s\n",
				shard["shard"], "主分片", shard["docs"], shard["node"])
		}
	}
	fmt.Println()

	// 用自定义 routing 把文档强制写到指定分片
	fmt.Println("   自定义路由（routing）— 把相关数据写到同一分片，减少跨分片查询：")
	brands := []string{"Apple", "Samsung", "Xiaomi"}
	for _, brand := range brands {
		body := fmt.Sprintf(`{"name":"test","brand":"%s","price":999}`, brand)
		res, _ := es.Index(
			indexName,
			strings.NewReader(body),
			es.Index.WithContext(ctx),
			es.Index.WithRouting(brand), // 按品牌路由，同品牌数据在同一分片
			es.Index.WithRefresh("true"),
		)
		res.Body.Close()
		fmt.Printf("   品牌 %-10s → 使用 routing=%s 写入\n", brand, brand)
	}
	fmt.Println()
}

// ----------------------------------------
// 3. 搜索偏好 preference
// ----------------------------------------
func searchPreference(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【3】搜索偏好（preference）：")
	fmt.Println()

	preferences := []struct {
		name  string
		value string
		desc  string
	}{
		{"默认", "", "随机选择主分片或副本（负载均衡）"},
		{"本地优先", "_local", "优先查询本节点的分片"},
		{"固定会话", "user_123", "同一个值总是查同一组分片（会话一致性）"},
	}

	query := `{"query":{"match_all":{}},"size":1}`

	for _, pref := range preferences {
		start := time.Now()

		opts := []func(*esapi.SearchRequest){
			es.Search.WithContext(ctx),
			es.Search.WithIndex(indexName),
			es.Search.WithBody(strings.NewReader(query)),
		}
		if pref.value != "" {
			opts = append(opts, es.Search.WithPreference(pref.value))
		}

		res, err := es.Search(opts...)
		if err != nil || res.IsError() {
			log.Printf("搜索失败: %s", res.String())
			continue
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		total := result["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)
		fmt.Printf("   %-10s preference=%-12s 耗时:%-8v 命中:%.0f条  说明:%s\n",
			pref.name, pref.value, time.Since(start), total, pref.desc)
	}
	fmt.Println()
}

// ----------------------------------------
// 4. 负载均衡验证
// ----------------------------------------
func loadBalanceDemo(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【4】负载均衡验证 — 多次查询请求分散到不同节点：")
	fmt.Println()

	// 直接连三个不同端口，模拟请求打到不同节点
	ports := []struct {
		port string
		name string
	}{
		{"9200", "es01(master)"},
		{"9201", "es02"},
		{"9202", "es03"},
	}

	query := `{"query":{"match_all":{}}}`

	for _, p := range ports {
		cfg := elasticsearch.Config{
			Addresses: []string{"http://localhost:" + p.port},
		}
		client, _ := elasticsearch.NewClient(cfg)

		start := time.Now()
		res, err := client.Search(
			client.Search.WithIndex(indexName),
			client.Search.WithBody(strings.NewReader(query)),
		)
		cost := time.Since(start)

		if err != nil || res.IsError() {
			fmt.Printf("   请求 %-15s → ❌ 失败\n", p.name)
			continue
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		total := result["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)
		fmt.Printf("   请求 %-15s → ✅ 成功 耗时:%-8v 命中:%.0f条\n",
			p.name, cost, total)
	}

	fmt.Println()
	fmt.Println("   结论：任意节点都能接收请求并返回完整结果")
	fmt.Println("   每个节点都充当协调节点，把请求转发给相关分片，汇总结果返回")
}
