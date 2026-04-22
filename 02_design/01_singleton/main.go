package main

import (
	"fmt"
	"sync"
)

// ========================================
// 单例模式 Singleton Pattern
// 场景：全局唯一的数据库连接管理器
// ========================================

// ----------------------------------------
// 方式一：饿汉式 —— 包加载时立即初始化
// ----------------------------------------

type EagerDB struct {
	dsn string
}

// 程序启动时就初始化，简单但无法懒加载
var eagerInstance = &EagerDB{dsn: "mysql://localhost:3306/eager"}

func GetEagerDB() *EagerDB {
	return eagerInstance
}

func (db *EagerDB) Query(sql string) string {
	return fmt.Sprintf("[EagerDB][%s] exec: %s", db.dsn, sql)
}

// ----------------------------------------
// 方式二：懒汉式 —— 加锁，每次调用都有锁开销
// ----------------------------------------

type LazyDB struct {
	dsn string
}

var (
	lazyInstance *LazyDB
	lazyMu       sync.Mutex
)

func GetLazyDB() *LazyDB {
	lazyMu.Lock()
	defer lazyMu.Unlock()

	if lazyInstance == nil {
		fmt.Println("  [LazyDB] 初始化中...")
		lazyInstance = &LazyDB{dsn: "mysql://localhost:3306/lazy"}
	}
	return lazyInstance
}

func (db *LazyDB) Query(sql string) string {
	return fmt.Sprintf("[LazyDB][%s] exec: %s", db.dsn, sql)
}

// ----------------------------------------
// 方式三：sync.Once —— Go 推荐写法 ✅
// ----------------------------------------

type DB struct {
	dsn string
}

var (
	dbInstance *DB
	dbOnce     sync.Once
)

func GetDB() *DB {
	dbOnce.Do(func() {
		// 这里只会执行一次，并发安全
		fmt.Println("  [DB] 初始化中（只会打印一次）...")
		dbInstance = &DB{dsn: "mysql://localhost:3306/prod"}
	})
	return dbInstance
}

func (db *DB) Query(sql string) string {
	return fmt.Sprintf("[DB][%s] exec: %s", db.dsn, sql)
}

// ========================================
// main：演示三种方式 + 并发验证
// ========================================

func main() {
	fmt.Println("=== 方式一：饿汉式 ===")
	db1 := GetEagerDB()
	db2 := GetEagerDB()
	fmt.Println(db1.Query("SELECT 1"))
	fmt.Printf("同一个实例？%v (指针: %p vs %p)\n\n", db1 == db2, db1, db2)

	fmt.Println("=== 方式二：懒汉式（加锁）===")
	ldb1 := GetLazyDB()
	ldb2 := GetLazyDB() // 第二次不会再打印"初始化中"
	fmt.Println(ldb1.Query("SELECT 2"))
	fmt.Printf("同一个实例？%v\n\n", ldb1 == ldb2)

	fmt.Println("=== 方式三：sync.Once（推荐）===")
	// 并发调用 10 次，验证只初始化一次
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			db := GetDB()
			fmt.Printf("  goroutine %d 拿到: %p\n", id, db)
		}(i)
	}
	wg.Wait()

	d1 := GetDB()
	d2 := GetDB()
	fmt.Printf("\n同一个实例？%v\n", d1 == d2)
	fmt.Println(d1.Query("SELECT 3"))
}
