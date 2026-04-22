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
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}

	ctx := context.Background()
	indexName := "products"

	// 1. 创建索引（带 Mapping）
	createIndex(es, ctx, indexName)

	// 2. 查看索引信息
	getIndex(es, ctx, indexName)

	// 3. 删除索引
	deleteIndex(es, ctx, indexName)
}

// ----------------------------------------
// 创建索引
// ----------------------------------------
func createIndex(es *elasticsearch.Client, ctx context.Context, indexName string) {
	// Mapping 定义了每个字段的类型，类似数据库的 Schema
	mapping := `{
		"mappings": {
			"properties": {
				"name":        { "type": "text" },
				"brand":       { "type": "keyword" },
				"price":       { "type": "float" },
				"stock":       { "type": "integer" },
				"description": { "type": "text" },
				"created_at":  { "type": "date" }
			}
		}
	}`

	res, err := es.Indices.Create(
		indexName,
		es.Indices.Create.WithContext(ctx),
		es.Indices.Create.WithBody(strings.NewReader(mapping)),
		es.Indices.Create.WithPretty(),
	)
	if err != nil {
		log.Fatalf("创建索引失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("创建索引错误: %s", res.String())
	}

	fmt.Printf("✅ 索引 [%s] 创建成功\n", indexName)
}

// ----------------------------------------
// 查看索引信息
// ----------------------------------------
func getIndex(es *elasticsearch.Client, ctx context.Context, indexName string) {
	res, err := es.Indices.Get(
		[]string{indexName},
		es.Indices.Get.WithContext(ctx),
		es.Indices.Get.WithPretty(),
	)
	if err != nil {
		log.Fatalf("获取索引失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("获取索引错误: %s", res.String())
	}

	// 解析返回的 JSON
	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	// 只打印 mappings 部分，避免输出太长
	indexInfo := result[indexName].(map[string]interface{})
	mappings := indexInfo["mappings"]

	mappingJSON, _ := json.MarshalIndent(mappings, "", "  ")
	fmt.Printf("\n📋 索引 [%s] 的 Mapping：\n%s\n", indexName, string(mappingJSON))
}

// ----------------------------------------
// 删除索引
// ----------------------------------------
func deleteIndex(es *elasticsearch.Client, ctx context.Context, indexName string) {
	res, err := es.Indices.Delete(
		[]string{indexName},
		es.Indices.Delete.WithContext(ctx),
	)
	if err != nil {
		log.Fatalf("删除索引失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("删除索引错误: %s", res.String())
	}

	fmt.Printf("\n🗑️  索引 [%s] 删除成功\n", indexName)
}
