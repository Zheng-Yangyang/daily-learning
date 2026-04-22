# 08 - 搜索高亮

## 功能描述

演示 ES 高亮功能：默认 `<em>` 标签高亮、自定义 HTML 标签、多字段高亮（name 完整返回 + description 片段截取）。

## 核心代码

```go
// 1. 基础高亮（默认 <em> 标签）
{"query": {"match": {"description": "旗舰"}},
 "highlight": {"fields": {"description": {}}}}

// 2. 自定义高亮标签（前端 CSS 控制样式）
{"highlight": {
    "pre_tags":  ["<span class='highlight'>"],
    "post_tags": ["</span>"],
    "fields": {"description": {}}}}

// 3. 多字段高亮，分别控制截取策略
{"highlight": {
    "fields": {
        "name":        {"number_of_fragments": 0},    // 返回完整字段
        "description": {"fragment_size": 50,           // 片段最大50字符
                        "number_of_fragments": 1}       // 最多1个片段
    }}}
```

## 高亮结果位置

```go
// 高亮内容在 highlight 字段，不在 _source 里
highlight := h["highlight"].(map[string]interface{})
descHighlight := highlight["description"].([]interface{})
fmt.Println(descHighlight[0])  // → "苹果<em>旗舰</em>手机..."
```

## 涉及知识点

| 知识点 | 说明 |
|--------|------|
| `pre_tags` / `post_tags` | 自定义高亮标签，默认 `<em>` / `</em>` |
| `number_of_fragments: 0` | 返回整个字段内容，适合短字段（name） |
| `fragment_size` | 每个高亮片段的最大字符数，适合长字段（description） |
| `number_of_fragments` | 最多返回几个高亮片段 |
| 高亮缺失处理 | 不是每条命中都一定有高亮，需判断 `ok` |
