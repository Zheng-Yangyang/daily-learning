# 05 - Bool Query DSL

## 功能描述

演示 ES Bool Query 的四种子句：must（AND）、should（OR）、must_not（NOT）、filter（纯过滤不计分），以及组合查询的实际用法。

## 四种子句

```json
{
  "query": {
    "bool": {
      "must":     [...],  // AND — 必须满足，参与评分
      "should":   [...],  // OR  — 满足任意，参与评分
      "must_not": [...],  // NOT — 排除，不参与评分
      "filter":   [...]   // 纯过滤，不参与评分，结果可缓存（性能优）
    }
  }
}
```

## 典型组合查询

```go
// 业务场景：描述含「手机」+ 价格3000~9000 + 排除Samsung
query := `{
    "query": {
        "bool": {
            "must":     [{"match": {"description": "手机"}}],
            "filter":   [{"range": {"price": {"gte": 3000, "lte": 9000}}}],
            "must_not": [{"term": {"brand": "Samsung"}}]
        }
    },
    "sort": [{"price": "asc"}]
}`
```

## must vs filter 的区别

| 特性 | must | filter |
|------|------|--------|
| 影响 `_score` | ✅ 是 | ❌ 否 |
| 结果缓存 | ❌ 否 | ✅ 是 |
| 性能 | 普通 | 更好 |
| 适用场景 | 全文搜索 | 条件过滤（价格、状态、分类） |

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `minimum_should_match` | should 中最少匹配几个条件 |
| `_score` 为 null | filter 模式不计算相关性评分 |
| 组合嵌套 | bool 内可嵌套 bool，实现复杂逻辑 |
