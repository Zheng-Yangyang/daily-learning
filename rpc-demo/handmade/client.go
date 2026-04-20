package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// ============================================================
// RPC Client
// 核心职责：
// 1. 维护连接池（简化版：单连接）
// 2. 发送请求，等待响应
// 3. 用 seq 匹配异步响应（支持并发调用）
// ============================================================

type Client struct {
	conn    net.Conn
	codec   *Codec
	seq     uint64 // 自增请求序号
	mu      sync.Mutex
	pending map[uint64]*Call // 等待响应的请求
	reader  *bufio.Reader
}

// Call 代表一次 RPC 调用
type Call struct {
	seq   uint64
	reply interface{}
	err   error
	done  chan struct{} // 调用完成时关闭这个 channel
}

func NewClient(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("连接服务端失败: %v", err)
	}

	c := &Client{
		conn:    conn,
		codec:   &Codec{},
		pending: make(map[uint64]*Call),
		reader:  bufio.NewReader(conn),
	}

	// 启动后台 goroutine 持续读取响应
	go c.readResponses()

	return c, nil
}

// Call 发起一次同步 RPC 调用
func (c *Client) Call(serviceMethod string, args interface{}) (interface{}, error) {
	// 分配唯一序号
	seq := atomic.AddUint64(&c.seq, 1)

	// 注册这次调用，等待响应
	call := &Call{
		seq:  seq,
		done: make(chan struct{}),
	}
	c.mu.Lock()
	c.pending[seq] = call
	c.mu.Unlock()

	// 序列化并发送请求
	req := &Request{
		ServiceMethod: serviceMethod,
		Seq:           seq,
		Args:          args,
	}
	data, err := c.codec.Encode(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	c.mu.Lock()
	_, err = c.conn.Write(data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	fmt.Printf("[Client] 发送请求: %s (seq=%d)\n", serviceMethod, seq)

	// 阻塞等待响应
	<-call.done

	return call.reply, call.err
}

// readResponses 后台持续读取服务端响应
// 这是支持并发调用的关键：多个 Call 同时发出，响应可能乱序返回
// 用 seq 匹配对应的 Call，通知它完成
func (c *Client) readResponses() {
	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			fmt.Printf("[Client] 连接断开: %v\n", err)
			return
		}

		var resp Response
		if err := c.codec.Decode(line, &resp); err != nil {
			fmt.Printf("[Client] 解码响应失败: %v\n", err)
			continue
		}

		// 找到对应的等待中的调用
		c.mu.Lock()
		call, ok := c.pending[resp.Seq]
		if ok {
			delete(c.pending, resp.Seq)
		}
		c.mu.Unlock()

		if !ok {
			fmt.Printf("[Client] 收到未知 seq 的响应: %d\n", resp.Seq)
			continue
		}

		// 填充结果，通知调用方
		if resp.Error != "" {
			call.err = fmt.Errorf(resp.Error)
		} else {
			call.reply = resp.Reply
		}
		close(call.done) // 关闭 channel，唤醒等待的 Call()
	}
}

func (c *Client) Close() {
	c.conn.Close()
}
