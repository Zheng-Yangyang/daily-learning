package main

import (
	"fmt"
	"sync"
	"time"
)

// call 代表一次正在进行的请求
type call struct {
	wg  sync.WaitGroup
	val string
	err error
}

// SingleFlight 核心结构
type SingleFlight struct {
	mu sync.Mutex
	m  map[string]*call
}

func (sf *SingleFlight) Do(key string, fn func() (string, error)) (string, error) {
	sf.mu.Lock()

	if sf.m == nil {
		sf.m = make(map[string]*call)
	}

	// 如果已经有相同 key 的请求在飞行中，等待它完成并复用结果
	if c, ok := sf.m[key]; ok {
		sf.mu.Unlock()
		c.wg.Wait()         // 阻塞等待第一个请求完成
		return c.val, c.err // 复用第一个请求的结果
	}

	// 第一个请求：注册到 map，开始执行
	c := &call{}
	c.wg.Add(1)
	sf.m[key] = c
	sf.mu.Unlock()

	// 真正执行查询
	c.val, c.err = fn()
	c.wg.Done() // 通知所有等待者

	// 请求完成，从 map 中删除
	sf.mu.Lock()
	delete(sf.m, key)
	sf.mu.Unlock()

	return c.val, c.err
}

// 模拟数据库查询
var dbHitCount int
var dbMu sync.Mutex

func queryDB(key string) (string, error) {
	dbMu.Lock()
	dbHitCount++
	dbMu.Unlock()

	time.Sleep(100 * time.Millisecond) // 模拟查询耗时
	return fmt.Sprintf("result_of_%s", key), nil
}

func main() {
	const requestNum = 100
	key := "key_foo"

	// ======= 没有保护的情况 =======
	fmt.Println("=== without singleflight ===")
	dbHitCount = 0
	var wg1 sync.WaitGroup
	for i := 0; i < requestNum; i++ {
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			queryDB(key)
		}()
	}
	wg1.Wait()
	fmt.Printf("hit db %d times\n\n", dbHitCount)

	// ======= singleflight 保护 =======
	fmt.Println("=== with singleflight ===")
	dbHitCount = 0
	sf := &SingleFlight{}
	var wg2 sync.WaitGroup
	results := make([]string, requestNum)

	for i := 0; i < requestNum; i++ {
		wg2.Add(1)
		i := i
		go func() {
			defer wg2.Done()
			val, _ := sf.Do(key, func() (string, error) {
				return queryDB(key)
			})
			results[i] = val
		}()
	}
	wg2.Wait()

	fmt.Printf("hit db %d times\n", dbHitCount)
	fmt.Printf("all %d requests got: %s\n", requestNum, results[0])
}
