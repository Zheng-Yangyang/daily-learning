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

type Product struct {
	Name        string  `json:"name"`
	Brand       string  `json:"brand"`
	Price       float64 `json:"price"`
	Description string  `json:"description"`
	Category    string  `json:"category,omitempty"` // 新增字段
}

const (
	oldIndex = "products_v1"
	newIndex = "products_v2"
)

func main() {
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}
	ctx := context.Background()

	// 1. 创建旧索引并写入数据
	setupOldIndex(es, ctx)

	// 2. 查看旧索引数据
	fmt.Println("【1】旧索引数据：")
	searchAll(es, ctx, oldIndex)

	// 3. 创建新索引（修改了 Mapping）
	setupNewIndex(es, ctx)

	// 4. 执行 reindex
	doReindex(es, ctx)

	// 5. 查看新索引数据
	fmt.Println("【3】新索引数据（reindex 后）：")
	searchAll(es, ctx, newIndex)

	// 6. 用别名实现零停机切换
	aliasSwitch(es, ctx)

	// 7. 通过别名查询
	fmt.Println("【5】通过别名查询（透明切换）：")
	searchAll(es, ctx, "products")
}

// ----------------------------------------
// 创建旧索引 products_v1
// ----------------------------------------
func setupOldIndex(es *elasticsearch.Client, ctx context.Context) {
	// 彻底清理：删除所有可能残留的索引
	es.Indices.Delete([]string{oldIndex}, es.Indices.Delete.WithContext(ctx))
	es.Indices.Delete([]string{newIndex}, es.Indices.Delete.WithContext(ctx))
	es.Indices.Delete([]string{"products"}, es.Indices.Delete.WithContext(ctx))

	// 旧 Mapping
	mapping := `{
		"mappings": {
			"properties": {
				"name":        { "type": "keyword" },
				"brand":       { "type": "keyword" },
				"price":       { "type": "float" },
				"description": { "type": "text", "analyzer": "standard" }
			}
		}
	}`
	res, _ := es.Indices.Create(oldIndex,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()

	products := []Product{
		{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999,
			Description: "苹果旗舰手机，搭载A17 Pro芯片"},
		{Name: "Galaxy S24 Ultra", Brand: "Samsung", Price: 9999,
			Description: "三星旗舰手机，骁龙8Gen3处理器"},
		{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999,
			Description: "小米旗舰手机，徕卡影像"},
		{Name: "MacBook Pro 14", Brand: "Apple", Price: 14999,
			Description: "苹果旗舰笔记本，M3芯片"},
		{Name: "小米平板6", Brand: "Xiaomi", Price: 2999,
			Description: "小米平板，骁龙870"},
	}

	for _, p := range products {
		body, _ := json.Marshal(p)
		res, _ := es.Index(oldIndex, strings.NewReader(string(body)),
			es.Index.WithContext(ctx))
		res.Body.Close()
	}
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(oldIndex))
	fmt.Printf("✅ 旧索引 [%s] 创建完成，共 %d 条\n\n", oldIndex, len(products))
}

// ----------------------------------------
// 创建新索引 products_v2（变更了 Mapping）
// ----------------------------------------
func setupNewIndex(es *elasticsearch.Client, ctx context.Context) {
	// 新 Mapping 改动：
	// 1. description 改用 ik_smart 分词
	// 2. 新增 category 字段
	mapping := `{
		"mappings": {
			"properties": {
				"name":        { "type": "keyword" },
				"brand":       { "type": "keyword" },
				"price":       { "type": "float" },
				"description": { "type": "text", "analyzer": "ik_smart" },
				"category":    { "type": "keyword" }
			}
		}
	}`
	res, _ := es.Indices.Create(newIndex,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()
	fmt.Printf("✅ 新索引 [%s] 创建完成（ik_smart分词 + category字段）\n\n", newIndex)
}

// ----------------------------------------
// 执行 reindex
// ----------------------------------------
func doReindex(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【2】执行 reindex...")

	reindexBody := fmt.Sprintf(`{
		"source": { "index": "%s" },
		"dest":   { "index": "%s" },
		"script": {
			"lang": "painless",
			"source": "if (ctx._source.brand == 'Apple') { ctx._source.category = 'apple_eco'; } else if (ctx._source.containsKey('name') && ctx._source.name.contains('Pro')) { ctx._source.category = 'flagship'; } else { ctx._source.category = 'others'; }"
		}
	}`, oldIndex, newIndex)

	start := time.Now()
	res, err := es.Reindex(
		strings.NewReader(reindexBody),
		es.Reindex.WithContext(ctx),
	)
	if err != nil || res.IsError() {
		log.Fatalf("reindex 失败: %s", res.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	total := result["total"].(float64)
	created := result["created"].(float64)
	took := time.Since(start)

	fmt.Printf("   总数据: %.0f 条\n", total)
	fmt.Printf("   成功迁移: %.0f 条\n", created)
	fmt.Printf("   耗时: %v\n\n", took)

	// reindex 完成后刷新新索引
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(newIndex))
}

// ----------------------------------------
// 别名切换 — 零停机迁移核心
// ----------------------------------------
func aliasSwitch(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【4】别名切换（零停机迁移）：")

	aliasBody := fmt.Sprintf(`{
		"actions": [
			{ "remove": { "index": "%s", "alias": "products" } },
			{ "add":    { "index": "%s", "alias": "products" } }
		]
	}`, oldIndex, newIndex)

	res, err := es.Indices.UpdateAliases(
		strings.NewReader(aliasBody),
		es.Indices.UpdateAliases.WithContext(ctx),
	)
	if err != nil || res.IsError() {
		addBody := fmt.Sprintf(`{
			"actions": [
				{ "add": { "index": "%s", "alias": "products" } }
			]
		}`, newIndex)
		res2, _ := es.Indices.UpdateAliases(strings.NewReader(addBody))
		res2.Body.Close()
	} else {
		res.Body.Close()
	}

	// 别名切换后刷新
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(newIndex))

	fmt.Printf("   别名 [products] 已指向新索引 [%s]\n\n", newIndex)
}

// ----------------------------------------
// 查询索引所有数据
// ----------------------------------------
func searchAll(es *elasticsearch.Client, ctx context.Context, index string) {
	query := `{
		"query": { "match_all": {} },
		"size": 10
	}`

	res, err := es.Search(
		es.Search.WithContext(ctx),
		es.Search.WithIndex(index),
		es.Search.WithBody(strings.NewReader(query)),
	)
	if err != nil || res.IsError() {
		log.Fatalf("搜索失败: %s", res.String())
	}
	defer res.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	hits := result["hits"].(map[string]interface{})
	hitList := hits["hits"].([]interface{})

	for _, hit := range hitList {
		h := hit.(map[string]interface{})
		source := h["_source"].(map[string]interface{})
		category := "-"
		if v, ok := source["category"]; ok && v != nil {
			category = v.(string)
		}
		fmt.Printf("   - %-18s | %-10s | ¥%.0f | 分类: %s\n",
			source["name"], source["brand"], source["price"], category)
	}
	fmt.Println()
}
