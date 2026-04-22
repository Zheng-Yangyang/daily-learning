# 06 - 聚合分析

## 功能描述

演示 ES 聚合的常用类型：terms 按字段分组（类似 GROUP BY）、stats 数值统计、range 区间分桶、嵌套聚合（品牌 + 价格统计），以及聚合 + 查询过滤组合。

## 五种聚合

```json
// 1. terms — 按品牌分组计数（GROUP BY brand）
{"size":0, "aggs": {"by_brand": {"terms": {"field": "brand"}}}}

// 2. stats — 价格统计（min/max/avg/sum/count）
{"size":0, "aggs": {"price_stats": {"stats": {"field": "price"}}}}

// 3. range — 按价格区间分桶
{"aggs": {"price_range": {"range": {"field": "price",
  "ranges": [{"to": 5000}, {"from": 5000, "to": 10000}, {"from": 10000}]}}}}

// 4. 嵌套聚合 — 每个品牌的平均/最高/最低价
{"aggs": {"by_brand": {"terms": {"field": "brand"},
  "aggs": {"avg_price": {"avg": {"field": "price"}}}}}}

// 5. query + aggs — 先过滤再聚合（只统计手机类）
{"query": {"match": {"description": "手机"}},
 "aggs": {"by_brand": {"terms": {"field": "brand"}}}}
```

## 关键参数

```
"size": 0 → 不返回原始文档，只返回聚合结果（节省带宽）
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| Bucket 聚合 | terms/range 把文档分到各个桶（bucket） |
| Metric 聚合 | stats/avg/max/min 对每个桶内文档计算指标 |
| 嵌套聚合 | 桶内嵌套 Metric，等价于 SQL `GROUP BY + AVG` |
| `order._count` | terms 按文档数降序排列 |
