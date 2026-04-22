# 11 - Bulk 批量操作

## 功能描述

演示 ES Bulk API：逐条写入 vs 批量写入的性能对比，bulk 混合操作（增/改/删），以及分批写入大数据量（每批 200 条）。

## Bulk 请求体格式

```
每个文档需要两行（Newline-Delimited JSON）：
第一行：action meta（操作类型 + 目标索引 + 可选 ID）
第二行：文档内容（delete 操作没有第二行）
```

```go
// 构造 bulk 请求体
var buf bytes.Buffer

// index（新增/覆盖）
buf.WriteString(`{"index":{"_index":"products"}}` + "\n")
buf.Write(body)
buf.WriteByte('\n')

// update（局部更新）
buf.WriteString(`{"update":{"_index":"products","_id":"p001"}}` + "\n")
buf.WriteString(`{"doc":{"price":7999}}` + "\n")

// delete（删除，无需第二行）
buf.WriteString(`{"delete":{"_index":"products","_id":"p002"}}` + "\n")

// 发送请求
es.Bulk(&buf, es.Bulk.WithRefresh("true"))
```

## 性能对比

```
逐条写入 100 条 vs 批量写入 100 条：
逐条写入：~500ms（每条一次 HTTP 往返）
批量写入：~50ms  （一次 HTTP 请求）
性能提升：约 10x
```

## 分批写入策略

```go
// 实际生产：按每批 200~1000 条或 5~10MB 切分
for i := 0; i < total; i += batchSize {
    batch := products[i : i+batchSize]
    // 构造并发送本批 bulk 请求
}
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| NDJSON 格式 | 每行独立 JSON，末尾必须有换行符 `\n` |
| `errors` 字段 | bulk 响应中 `errors=true` 表示有部分文档失败 |
| `items` 数组 | 每条操作的结果（结果字段：`index`/`update`/`delete`） |
| 批大小选择 | 建议 5~10MB 或 500~1000 条，过大会占用内存和超时 |
