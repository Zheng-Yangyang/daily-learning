package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

type Product struct {
	Name        string  `json:"name"`
	Brand       string  `json:"brand"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
	Description string  `json:"description"`
}

const indexName = "products"

func main() {
	es, err := elasticsearch.NewDefaultClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %s", err)
	}
	ctx := context.Background()

	setupIndex(es, ctx)
	fmt.Println("========================================")

	// 1. 逐条写入 vs 批量写入性能对比
	benchmarkSingleVsBulk(es, ctx)

	// 2. bulk 混合操作（增/改/删）
	bulkMixedOperations(es, ctx)

	// 3. 分批写入大数据量
	bulkBatch(es, ctx)
}

// ----------------------------------------
// 1. 性能对比
// ----------------------------------------
func benchmarkSingleVsBulk(es *elasticsearch.Client, ctx context.Context) {
	products := generateProducts(100)

	// 逐条写入
	start := time.Now()
	for _, p := range products {
		body, _ := json.Marshal(p)
		res, _ := es.Index(indexName, strings.NewReader(string(body)),
			es.Index.WithContext(ctx))
		res.Body.Close()
	}
	singleCost := time.Since(start)

	// 清空索引重来
	es.DeleteByQuery(
		[]string{indexName},
		strings.NewReader(`{"query":{"match_all":{}}}`),
	)
	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))

	// 批量写入
	start = time.Now()
	bulkInsert(es, ctx, products)
	bulkCost := time.Since(start)

	fmt.Println("【1】逐条写入 vs 批量写入（100条）：")
	fmt.Printf("   逐条写入耗时: %v\n", singleCost)
	fmt.Printf("   批量写入耗时: %v\n", bulkCost)
	fmt.Printf("   性能提升: %.1fx\n\n", float64(singleCost)/float64(bulkCost))
}

// ----------------------------------------
// 核心：构造 bulk 请求体
// ----------------------------------------
func bulkInsert(es *elasticsearch.Client, ctx context.Context, products []Product) {
	var buf bytes.Buffer

	for _, p := range products {
		// 每个文档需要两行：
		// 第一行：action meta，告诉 ES 做什么操作
		// 第二行：文档内容
		meta := `{"index":{"_index":"` + indexName + `"}}` + "\n"
		buf.WriteString(meta)

		body, _ := json.Marshal(p)
		buf.Write(body)
		buf.WriteByte('\n')
	}

	res, err := es.Bulk(
		&buf,
		es.Bulk.WithContext(ctx),
		es.Bulk.WithRefresh("true"),
	)
	if err != nil || res.IsError() {
		log.Fatalf("bulk 写入失败: %s", res.String())
	}
	defer res.Body.Close()

	// 检查是否有部分文档失败
	var result map[string]interface{}
	json.NewDecoder(res.Body).Decode(&result)
	if result["errors"].(bool) {
		log.Printf("⚠️  bulk 有部分失败，需要检查 items")
	}
}

// ----------------------------------------
// 2. bulk 混合操作
// ----------------------------------------
func bulkMixedOperations(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【2】bulk 混合操作（增/改/删）：")

	// 先插入几条有固定 ID 的文档
	fixedProducts := []struct {
		ID      string
		Product Product
	}{
		{"p001", Product{Name: "iPhone 15 Pro", Brand: "Apple", Price: 8999, Stock: 50}},
		{"p002", Product{Name: "Galaxy S24", Brand: "Samsung", Price: 6999, Stock: 80}},
		{"p003", Product{Name: "小米14", Brand: "Xiaomi", Price: 3999, Stock: 200}},
	}

	var buf bytes.Buffer
	for _, item := range fixedProducts {
		meta := fmt.Sprintf(`{"index":{"_index":"%s","_id":"%s"}}`, indexName, item.ID) + "\n"
		buf.WriteString(meta)
		body, _ := json.Marshal(item.Product)
		buf.Write(body)
		buf.WriteByte('\n')
	}
	res, _ := es.Bulk(&buf, es.Bulk.WithRefresh("true"))
	res.Body.Close()
	fmt.Println("   初始数据插入完成（p001/p002/p003）")

	// 混合操作：更新 p001，删除 p002，新增 p004
	buf.Reset()

	// update p001 — 降价
	buf.WriteString(fmt.Sprintf(`{"update":{"_index":"%s","_id":"p001"}}`, indexName) + "\n")
	buf.WriteString(`{"doc":{"price":7999,"stock":30}}` + "\n")

	// delete p002
	buf.WriteString(fmt.Sprintf(`{"delete":{"_index":"%s","_id":"p002"}}`, indexName) + "\n")

	// index p004 — 新增
	buf.WriteString(fmt.Sprintf(`{"index":{"_index":"%s","_id":"p004"}}`, indexName) + "\n")
	newProduct := Product{Name: "小米14 Pro", Brand: "Xiaomi", Price: 4999, Stock: 100}
	body, _ := json.Marshal(newProduct)
	buf.Write(body)
	buf.WriteByte('\n')

	res2, err := es.Bulk(&buf, es.Bulk.WithContext(ctx), es.Bulk.WithRefresh("true"))
	if err != nil || res2.IsError() {
		log.Fatalf("bulk 混合操作失败: %s", res2.String())
	}
	defer res2.Body.Close()

	var mixResult map[string]interface{}
	json.NewDecoder(res2.Body).Decode(&mixResult)

	items := mixResult["items"].([]interface{})
	for _, item := range items {
		itemMap := item.(map[string]interface{})
		for op, detail := range itemMap {
			d := detail.(map[string]interface{})
			fmt.Printf("   %-8s _id=%-6s result=%s\n",
				op, d["_id"], d["result"])
		}
	}
	fmt.Println()
}

// ----------------------------------------
// 3. 分批写入大数据量
// ----------------------------------------
func bulkBatch(es *elasticsearch.Client, ctx context.Context) {
	fmt.Println("【3】分批写入 1000 条数据：")

	const (
		total     = 1000
		batchSize = 200 // 每批 200 条，实际生产建议 500~1000 条或 5~10MB
	)

	products := generateProducts(total)
	start := time.Now()
	successCount := 0

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}
		batch := products[i:end]

		var buf bytes.Buffer
		for _, p := range batch {
			buf.WriteString(`{"index":{"_index":"` + indexName + `"}}` + "\n")
			body, _ := json.Marshal(p)
			buf.Write(body)
			buf.WriteByte('\n')
		}

		res, err := es.Bulk(&buf, es.Bulk.WithContext(ctx))
		if err != nil || res.IsError() {
			log.Printf("第 %d 批失败: %s", i/batchSize+1, res.String())
			res.Body.Close()
			continue
		}

		var result map[string]interface{}
		json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()

		if !result["errors"].(bool) {
			successCount += len(batch)
		}

		fmt.Printf("   第 %d 批写入完成（%d~%d），累计成功 %d 条\n",
			i/batchSize+1, i+1, end, successCount)
	}

	es.Indices.Refresh(es.Indices.Refresh.WithIndex(indexName))
	fmt.Printf("\n   总耗时: %v，成功写入: %d 条\n\n", time.Since(start), successCount)
}

// ----------------------------------------
// 生成测试数据
// ----------------------------------------
func generateProducts(n int) []Product {
	brands := []string{"Apple", "Samsung", "Xiaomi", "Huawei", "OPPO"}
	products := make([]Product, n)
	for i := 0; i < n; i++ {
		brand := brands[rand.Intn(len(brands))]
		products[i] = Product{
			Name:  fmt.Sprintf("%s Product-%d", brand, i+1),
			Brand: brand,
			Price: float64(1000 + rand.Intn(14000)),
			Stock: rand.Intn(500),
		}
	}
	return products
}

// ----------------------------------------
// 初始化索引
// ----------------------------------------
func setupIndex(es *elasticsearch.Client, ctx context.Context) {
	es.Indices.Delete([]string{indexName}, es.Indices.Delete.WithContext(ctx))
	mapping := `{
		"mappings": {
			"properties": {
				"name":  { "type": "keyword" },
				"brand": { "type": "keyword" },
				"price": { "type": "float" },
				"stock": { "type": "integer" }
			}
		}
	}`
	res, _ := es.Indices.Create(indexName,
		es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res.Body.Close()
	fmt.Println("✅ 索引初始化完成")
}
