package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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

	// 1. 创建带分片配置的索引
	createShardedIndex(es, ctx)

	// 2. 查看分片分布
	showShardDistribution(es, ctx)

	// 3. 写入数据，观察路由规则
	writeAndCheckRouting(es, ctx)

	// 4. 查看分片详细状态
	showShardStats(es, ctx)
}

// ----------------------------------------
// 1. 创建索引：3个主分片，1个副本
// ----------------------------------------
func createShardedIndex(es *elasticsearch.Client, ctx context.Context) {
	es.Indices.Delete([]string{indexName})

	// number_of_shards:   主分片数，创建后不能修改
	// number_of_replicas: 副本数，可以随时修改
	settings := `{
		"settings": {
			"number_of_shards":   3,
			"number_of_replicas": 1
		},
		"mappings": {
			"properties": {
				"name":  { "type": "keyword" },
				"brand": { "type": "keyword" },
				"price": { "type": "float" }
			}
		}
	}`

	res, err := es.Indices.Create(indexName,
		es.Indices.Create.WithContext(ctx),
		es.Indices.Create.WithBody(strings.NewReader(settings)),
	)
	if err != nil || res.IsError() {
		log.Fatalf("创建索引失败: %s", res.String())
	}
	res.Body.Close()

	fmt.Println("✅ 索引创建完成")
	fmt.Println("   主分片数: 3（数据分散存储在3个分片上）")
	fmt.Println("   副本数:   1（每个主分片有1个副本，共6个分片）")
	fmt.Println()
}

// ----------------------------------------
// 2. 查看分片在节点上的分布
// ----------------------------------------
func showShardDistribution(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Cat.Shards(
		es.Cat.Shards.WithContext(ctx),
		es.Cat.Shards.WithIndex(indexName),
		es.Cat.Shards.WithFormat("json"),
		es.Cat.Shards.WithH("index", "shard", "prirep", "state", "docs", "node"),
	)
	if err != nil || res.IsError() {
		log.Fatalf("获取分片信息失败: %s", res.String())
	}
	defer res.Body.Close()

	var shards []map[string]interface{}
	json.NewDecoder(res.Body).Decode(&shards)

	fmt.Println("【1】分片分布（3主分片 + 3副本 = 6个分片）：")
	fmt.Printf("   %-6s %-6s %-8s %-4s %-8s\n", "分片", "类型", "状态", "文档数", "所在节点")
	fmt.Println("   " + strings.Repeat("-", 40))

	for _, shard := range shards {
		shardType := "副本"
		if shard["prirep"] == "p" {
			shardType = "主分片"
		}
		fmt.Printf("   %-6s %-6s %-8s %-4s %-8s\n",
			shard["shard"],
			shardType,
			shard["state"],
			shard["docs"],
			shard["node"],
		)
	}
	fmt.Println()
}

// ----------------------------------------
// 3. 写入数据，观察路由规则
// ----------------------------------------
func writeAndCheckRouting(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【2】写入数据，观察路由到哪个分片：")
	fmt.Println("   路由公式：shard = hash(id) % number_of_shards")
	fmt.Println()

	products := []struct {
		ID    string
		Name  string
		Brand string
		Price float64
	}{
		{"p001", "iPhone 15 Pro", "Apple", 8999},
		{"p002", "Galaxy S24", "Samsung", 6999},
		{"p003", "小米14 Pro", "Xiaomi", 4999},
		{"p004", "MacBook Pro", "Apple", 14999},
		{"p005", "小米平板6", "Xiaomi", 2999},
	}

	for _, p := range products {
		body := fmt.Sprintf(`{"name":"%s","brand":"%s","price":%v}`,
			p.Name, p.Brand, p.Price)

		res, err := es.Index(
			indexName,
			strings.NewReader(body),
			es.Index.WithDocumentID(p.ID),
			es.Index.WithContext(ctx),
			es.Index.WithRefresh("true"),
		)
		if err != nil || res.IsError() {
			log.Fatalf("写入失败: %s", res.String())
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		shard := result["_shards"].(map[string]interface{})
		fmt.Printf("   ID=%-6s → 分片#%v | 成功写入节点数: %.0f\n",
			p.ID,
			result["_primary_term"],
			shard["successful"],
		)
	}
	fmt.Println()

	// 写入后再看分片数据分布
	fmt.Println("【3】写入后分片数据分布：")
	showShardDistributionAfterWrite(es, ctx)
}

// ----------------------------------------
// 写入后查看每个分片的文档数
// ----------------------------------------
func showShardDistributionAfterWrite(es *elasticsearch.Client, ctx context.Context) {
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

	fmt.Printf("   %-6s %-8s %-6s %-8s\n", "分片", "类型", "文档数", "节点")
	fmt.Println("   " + strings.Repeat("-", 32))
	for _, shard := range shards {
		shardType := "副本"
		if shard["prirep"] == "p" {
			shardType = "主分片"
		}
		fmt.Printf("   %-6s %-8s %-6s %-8s\n",
			shard["shard"], shardType, shard["docs"], shard["node"])
	}
	fmt.Println()
}

// ----------------------------------------
// 4. 分片统计信息
// ----------------------------------------
func showShardStats(es *elasticsearch.Client, ctx context.Context) {
	res, err := es.Indices.Stats(
		es.Indices.Stats.WithContext(ctx),
		es.Indices.Stats.WithIndex(indexName),
	)
	if err != nil || res.IsError() {
		log.Fatalf("获取统计失败: %s", res.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	indices := result["indices"].(map[string]interface{})
	indexStats := indices[indexName].(map[string]interface{})
	total := indexStats["total"].(map[string]interface{})
	docs := total["docs"].(map[string]interface{})
	store := total["store"].(map[string]interface{})

	fmt.Println("【4】索引统计信息：")
	fmt.Printf("   总文档数: %.0f\n", docs["count"])
	fmt.Printf("   存储大小: %.0f bytes\n", store["size_in_bytes"])

	primaries := indexStats["primaries"].(map[string]interface{})
	primDocs := primaries["docs"].(map[string]interface{})
	fmt.Printf("   主分片文档数: %.0f（副本不计入）\n", primDocs["count"])
	fmt.Println()

	fmt.Println("   核心概念回顾：")
	fmt.Println("   • 主分片  — 数据写入的目标，数量创建后不可改")
	fmt.Println("   • 副本分片 — 主分片的完整拷贝，可提高读取性能和容灾")
	fmt.Println("   • 写入时  — 只写主分片，ES自动同步到副本")
	fmt.Println("   • 读取时  — 主分片和副本都可以响应查询（负载均衡）")
}
