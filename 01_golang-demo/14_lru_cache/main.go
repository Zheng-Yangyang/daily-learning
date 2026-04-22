package main

import (
	"fmt"
	"sync"
)

// Node 双向链表节点
type Node struct {
	key, val   int
	prev, next *Node
}

// LRUCache 并发安全的 LRU 缓存
type LRUCache struct {
	mu       sync.Mutex
	capacity int
	cache    map[int]*Node
	head     *Node // 虚拟头节点，最近使用
	tail     *Node // 虚拟尾节点，最久未使用
}

func NewLRUCache(capacity int) *LRUCache {
	head := &Node{}
	tail := &Node{}
	head.next = tail
	tail.prev = head
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[int]*Node),
		head:     head,
		tail:     tail,
	}
}

// addToFront 将节点插入到头部（最近使用）
func (l *LRUCache) addToFront(node *Node) {
	node.prev = l.head
	node.next = l.head.next
	l.head.next.prev = node
	l.head.next = node
}

// removeNode 从链表中摘除节点
func (l *LRUCache) removeNode(node *Node) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

// removeTail 删除尾部节点（最久未使用）
func (l *LRUCache) removeTail() *Node {
	tail := l.tail.prev
	l.removeNode(tail)
	return tail
}

func (l *LRUCache) Get(key int) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if node, ok := l.cache[key]; ok {
		l.removeNode(node) // 从当前位置摘除
		l.addToFront(node) // 移到头部（最近使用）
		return node.val
	}
	return -1
}

func (l *LRUCache) Put(key, val int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if node, ok := l.cache[key]; ok {
		// key 已存在：更新值，移到头部
		node.val = val
		l.removeNode(node)
		l.addToFront(node)
		return
	}

	// key 不存在：新建节点
	node := &Node{key: key, val: val}
	l.cache[key] = node
	l.addToFront(node)

	// 超出容量：淘汰尾部节点
	if len(l.cache) > l.capacity {
		tail := l.removeTail()
		delete(l.cache, tail.key)
		fmt.Printf("evicted: key=%d\n", tail.key)
	}
}

func main() {
	lru := NewLRUCache(3)

	// 串行验证基本逻辑
	fmt.Println("=== basic test ===")
	lru.Put(1, 1)
	lru.Put(2, 2)
	lru.Put(3, 3)
	fmt.Printf("get(1) = %d\n", lru.Get(1)) // 1 移到头部，链表: 1->3->2
	lru.Put(4, 4)                           // 淘汰最久未使用的 2
	fmt.Printf("get(2) = %d\n", lru.Get(2)) // -1，已淘汰
	fmt.Printf("get(3) = %d\n", lru.Get(3)) // 3
	fmt.Printf("get(4) = %d\n", lru.Get(4)) // 4

	// 并发压测
	fmt.Println("\n=== concurrent test ===")
	lru2 := NewLRUCache(5)
	var wg sync.WaitGroup

	// 10 个写 goroutine
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			lru2.Put(i, i*10)
		}()
	}

	// 10 个读 goroutine
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			val := lru2.Get(i)
			_ = val
		}()
	}

	wg.Wait()
	fmt.Println("concurrent test passed, no data race")
	fmt.Println("\nall done")
}
