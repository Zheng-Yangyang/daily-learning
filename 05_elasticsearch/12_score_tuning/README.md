# 12 - 评分调优

## 功能描述

演示 ES 相关性评分调优：默认 BM25 评分、boost 字段权重提升、function_score 销量加权、综合评分（销量 + 用户评分 + 爆款加权）。

## 四种评分策略

```go
// 1. 默认 BM25 — 基于词频、逆文档频率、字段长度
{"query": {"match": {"description": "旗舰手机"}}}

// 2. boost — name 字段命中权重是 description 的 3 倍
{"query": {"multi_match": {
    "query":  "旗舰手机",
    "fields": ["name^3", "description"]}}}

// 3. function_score 销量加权
// score = BM25分 * log(1 + sales)
{"query": {"function_score": {
    "query": {"match": {"description": "手机"}},
    "field_value_factor": {"field": "sales", "modifier": "log1p"},
    "boost_mode": "multiply"}}}

// 4. 综合评分（多个函数 + 爆款加权）
{"query": {"function_score": {
    "functions": [
        {"field_value_factor": {"field": "sales", "factor": 0.5}},
        {"field_value_factor": {"field": "rating", "factor": 2.0}},
        {"filter": {"range": {"sales": {"gte": 10000}}}, "weight": 3}
    ],
    "score_mode": "sum",    // functions 之间用加法合并
    "boost_mode": "replace" // 直接用 function 分替换 query 分
}}}
```

## 关键参数说明

| 参数 | 值 | 含义 |
|------|-----|------|
| `modifier` | `log1p` | `log(1 + value)`，平滑大数值 |
| `score_mode` | `sum/avg/max/multiply` | 多个 function 之间如何合并 |
| `boost_mode` | `replace/multiply/sum` | function 分和 query 分如何合并 |

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| BM25 | ES 默认相关性算法，考虑词频、文档频率、字段长度归一化 |
| `boost` | 搜索时临时提升某字段权重，不修改索引 |
| `function_score` | 将业务指标（销量、评分）融入搜索排名 |
| `field_value_factor` | 用文档字段值参与评分计算 |
