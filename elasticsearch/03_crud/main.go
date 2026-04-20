package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
)

// Product 对应 ES 文档结构
type Product struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Brand       string  `json:"brand"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
}

const indexName = "products"

func main() {
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}
	ctx := context.Background()

	// 先创建索引
	setupIndex(es, ctx)

	// 1. 新增文档
	id := createDocument(es, ctx)

	// 2. 查询文档
	getDocument(es, ctx, id)

	// 3. 更新文档
	updateDocument(es, ctx, id)

	// 4. 再次查询，确认更新
	getDocument(es, ctx, id)

	// 5. 删除文档
	deleteDocument(es, ctx, id)

	// 6. 确认删除
	getDocument(es, ctx, id)
}

// ----------------------------------------
// 初始化索引
// ----------------------------------------
func setupIndex(es *elasticsearch.Client, ctx context.Context) {
	// 先删除（忽略不存在的错误），再重建，保证干净的测试环境
	es.Indices.Delete([]string{indexName}, es.Indices.Delete.WithContext(ctx))

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
	)
	if err != nil || res.IsError() {
		log.Fatalf("创建索引失败: %s", res.String())
	}
	res.Body.Close()
	fmt.Println("✅ 索引初始化完成\n")
}

// ----------------------------------------
// Create — 新增文档
// ----------------------------------------
func createDocument(es *elasticsearch.Client, ctx context.Context) string {
	product := Product{
		Name:        "iPhone 15 Pro",
		Brand:       "Apple",
		Price:       8999.00,
		Stock:       100,
		Description: "苹果最新旗舰手机，搭载 A17 Pro 芯片",
		CreatedAt:   "2024-01-01",
	}

	body, _ := json.Marshal(product)

	res, err := es.Index(
		indexName,
		strings.NewReader(string(body)),
		es.Index.WithContext(ctx),
		es.Index.WithRefresh("true"), // 立即刷新，让文档马上可被搜索到
	)
	if err != nil {
		log.Fatalf("新增文档失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("新增文档错误: %s", res.String())
	}

	// 从响应中拿到 ES 自动生成的文档 ID
	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)
	id := result["_id"].(string)

	fmt.Printf("➕ Create — 文档创建成功，ID: %s\n", id)
	return id
}

// ----------------------------------------
// Read — 查询文档
// ----------------------------------------
func getDocument(es *elasticsearch.Client, ctx context.Context, id string) {
	res, err := es.Get(
		indexName,
		id,
		es.Get.WithContext(ctx),
	)
	if err != nil {
		log.Fatalf("查询文档失败: %s", err)
	}
	defer res.Body.Close()

	// 文档不存在时 IsError() 为 true（返回 404）
	if res.IsError() {
		fmt.Printf("🔍 Get    — 文档 [%s] 不存在（已删除）\n\n", id)
		return
	}

	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)

	// _source 里才是真正的文档内容
	source := result["_source"]
	sourceJSON, _ := json.MarshalIndent(source, "", "  ")
	fmt.Printf("🔍 Get    — 文档内容：\n%s\n\n", string(sourceJSON))
}

// ----------------------------------------
// Update — 更新文档（局部更新）
// ----------------------------------------
func updateDocument(es *elasticsearch.Client, ctx context.Context, id string) {
	// 用 doc 包裹，只更新指定字段，其他字段保持不变
	updateBody := `{
		"doc": {
			"price": 7999.00,
			"stock": 80
		}
	}`

	res, err := es.Update(
		indexName,
		id,
		strings.NewReader(updateBody),
		es.Update.WithContext(ctx),
		es.Update.WithRefresh("true"),
	)
	if err != nil {
		log.Fatalf("更新文档失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("更新文档错误: %s", res.String())
	}

	fmt.Printf("✏️  Update — 文档 [%s] 更新成功（price: 8999 → 7999，stock: 100 → 80）\n\n", id)
}

// ----------------------------------------
// Delete — 删除文档
// ----------------------------------------
func deleteDocument(es *elasticsearch.Client, ctx context.Context, id string) {
	res, err := es.Delete(
		indexName,
		id,
		es.Delete.WithContext(ctx),
		es.Delete.WithRefresh("true"),
	)
	if err != nil {
		log.Fatalf("删除文档失败: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		log.Fatalf("删除文档错误: %s", res.String())
	}

	fmt.Printf("🗑️  Delete — 文档 [%s] 删除成功\n\n", id)
}
