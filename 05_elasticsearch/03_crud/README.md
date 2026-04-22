# 03 - 文档 CRUD

## 功能描述

完整演示 ES 文档的增删改查：新增文档（ES 自动生成 ID）、按 ID 查询、局部更新指定字段、删除文档。

## 四种操作

```go
// Create — 新增文档，ES 返回自动生成的 _id
res, _ := es.Index(indexName, strings.NewReader(body),
    es.Index.WithRefresh("true"))  // 立即刷新，使文档马上可被搜索

// Read — 按 ID 查询，_source 里是文档内容
res, _ := es.Get(indexName, id)
// 文档不存在时 res.IsError() = true（404）

// Update — 局部更新（只修改指定字段，其他字段不变）
updateBody := `{"doc": {"price": 7999, "stock": 80}}`
es.Update(indexName, id, strings.NewReader(updateBody))

// Delete — 删除文档
es.Delete(indexName, id, es.Delete.WithRefresh("true"))
```

## 注意事项

```
WithRefresh("true")  → 写后立即刷新，文档马上可搜索（有性能开销）
WithRefresh("false") → 默认，等待下次自动刷新（约 1 秒）
WithRefresh("wait_for") → 等待刷新完成再返回（折中方案）
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `es.Index()` | 对应 PUT/POST /{index}/_doc，自动生成 ID |
| `es.Update()` | 对应 POST /{index}/_update/{id}，支持 `doc` 局部更新和 `script` |
| `_source` | 文档的实际内容存储在 `_source` 字段中 |
| 404 判断 | `res.IsError()` 包含 404，用来判断文档是否存在 |
