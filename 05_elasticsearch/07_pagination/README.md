# 07 - 分页查询

## 功能描述

对比两种分页方案：from/size 基础分页及其深度分页限制，以及 search_after 游标分页（推荐生产使用）。

## 方案一：from/size（浅分页）

```go
// 第 N 页（每页 5 条）
query := fmt.Sprintf(`{
    "query": {"match_all": {}},
    "sort":  [{"price": "asc"}, {"name": "asc"}],
    "from": %d,
    "size": 5
}`, page*5)
```

**限制**：from + size > 10000 时 ES 直接报错（`max_result_window` 默认 10000）。  
原因：ES 需要在每个分片上取 from+size 条再合并排序，深度分页内存开销极大。

## 方案二：search_after（游标分页）

```go
// 第一页不带游标
query := `{"sort": [{"price": "asc"}, {"name": "asc"}], "size": 5}`

// 后续页用上一页最后一条的 sort 值作为游标
searchAfter = lastHit["sort"].([]interface{})
query := fmt.Sprintf(`{
    "sort":         [{"price": "asc"}, {"name": "asc"}],
    "size":         5,
    "search_after": %s
}`, cursorJSON)
```

## 两种方案对比

| 特性 | from/size | search_after |
|------|-----------|--------------|
| 随机跳页 | ✅ 支持 | ❌ 不支持（只能顺序翻） |
| 深分页性能 | ❌ 差，超 10000 报错 | ✅ 恒定性能 |
| 适用场景 | 搜索结果前几页 | 无限滚动、数据导出 |

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `max_result_window` | 默认 10000，超过则拒绝请求 |
| sort 稳定性 | search_after 需要唯一排序键（如加 `_id`）避免游标不稳定 |
| keyword 排序 | text 字段不能排序，需用 keyword 类型 |
