package main

import (
	"encoding/json"
	"fmt"
)

// ============================================================
// RPC 消息格式
// 真实框架用 Thrift/Protobuf 二进制编码，我们用 JSON 方便理解
// ============================================================

// Request 调用方发出的请求
type Request struct {
	ServiceMethod string      `json:"service_method"` // 调用哪个方法，如 "UserService.GetUser"
	Seq           uint64      `json:"seq"`            // 请求序号，用于匹配响应
	Args          interface{} `json:"args"`           // 参数
}

// Response 服务端返回的响应
type Response struct {
	ServiceMethod string      `json:"service_method"`
	Seq           uint64      `json:"seq"`
	Reply         interface{} `json:"reply"` // 返回值
	Error         string      `json:"error"` // 错误信息，空字符串表示成功
}

// ============================================================
// 编解码：序列化和反序列化
// 真实框架这里是性能最敏感的部分，Kitex 用 Thrift 二进制编码
// ============================================================

type Codec struct{}

func (c *Codec) Encode(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("编码失败: %v", err)
	}
	return data, nil
}

func (c *Codec) Decode(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("解码失败: %v", err)
	}
	return nil
}
