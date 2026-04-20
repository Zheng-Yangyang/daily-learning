package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ========================================
// 代理模式 Proxy Pattern
//
// 场景：数据库查询服务
//
// 解决的问题：
//   控制对真实对象的访问，在访问前后插入额外逻辑
//   常见用途：
//     1. 缓存代理   —— 避免重复查询数据库
//     2. 保护代理   —— 权限控制
//     3. 虚拟代理   —— 延迟初始化（懒加载）
//
// 和装饰器的区别：
//   装饰器：增强功能，调用方知道在增强，主动套上去
//   代理：控制访问，调用方不感知代理的存在，透明替换
// ========================================

// ----------------------------------------
// 核心接口：UserRepository
// ----------------------------------------

type User struct {
	ID   int
	Name string
	Age  int
}

type UserRepository interface {
	FindByID(id int) (*User, error)
	FindAll() ([]*User, error)
	Save(user *User) error
}

// ----------------------------------------
// 真实对象：直连数据库
// ----------------------------------------

type DBUserRepository struct {
	dsn        string
	queryCount int // 统计真实查询次数
}

func NewDBUserRepository(dsn string) *DBUserRepository {
	fmt.Printf("  [DB] 建立连接: %s\n", dsn)
	return &DBUserRepository{dsn: dsn}
}

func (r *DBUserRepository) FindByID(id int) (*User, error) {
	r.queryCount++
	time.Sleep(20 * time.Millisecond) // 模拟数据库延迟
	fmt.Printf("  [DB] SELECT * FROM users WHERE id=%d (第%d次查询)\n", id, r.queryCount)

	// 模拟数据
	users := map[int]*User{
		1: {ID: 1, Name: "Alice", Age: 28},
		2: {ID: 2, Name: "Bob", Age: 32},
		3: {ID: 3, Name: "Carol", Age: 25},
	}
	if u, ok := users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("用户 %d 不存在", id)
}

func (r *DBUserRepository) FindAll() ([]*User, error) {
	r.queryCount++
	time.Sleep(50 * time.Millisecond) // 模拟慢查询
	fmt.Printf("  [DB] SELECT * FROM users (第%d次查询)\n", r.queryCount)
	return []*User{
		{ID: 1, Name: "Alice", Age: 28},
		{ID: 2, Name: "Bob", Age: 32},
		{ID: 3, Name: "Carol", Age: 25},
	}, nil
}

func (r *DBUserRepository) Save(user *User) error {
	r.queryCount++
	fmt.Printf("  [DB] INSERT/UPDATE user id=%d (第%d次查询)\n", user.ID, r.queryCount)
	return nil
}

// ----------------------------------------
// 代理一：缓存代理
// 对调用方完全透明，接口一致
// ----------------------------------------

type CacheProxy struct {
	real     UserRepository
	cache    map[int]*User
	allCache []*User
	mu       sync.RWMutex
	ttl      map[int]time.Time // 缓存过期时间
}

func NewCacheProxy(real UserRepository) *CacheProxy {
	return &CacheProxy{
		real:  real,
		cache: make(map[int]*User),
		ttl:   make(map[int]time.Time),
	}
}

func (p *CacheProxy) FindByID(id int) (*User, error) {
	p.mu.RLock()
	if user, ok := p.cache[id]; ok {
		if time.Now().Before(p.ttl[id]) { // 未过期
			p.mu.RUnlock()
			fmt.Printf("  [Cache] HIT  id=%d ✓\n", id)
			return user, nil
		}
	}
	p.mu.RUnlock()

	// 缓存未命中，查真实数据库
	fmt.Printf("  [Cache] MISS id=%d，查询数据库...\n", id)
	user, err := p.real.FindByID(id)
	if err != nil {
		return nil, err
	}

	// 写入缓存，TTL 100ms（演示用，实际会是分钟级）
	p.mu.Lock()
	p.cache[id] = user
	p.ttl[id] = time.Now().Add(100 * time.Millisecond)
	p.mu.Unlock()

	return user, nil
}

func (p *CacheProxy) FindAll() ([]*User, error) {
	p.mu.RLock()
	if p.allCache != nil {
		p.mu.RUnlock()
		fmt.Println("  [Cache] HIT  FindAll ✓")
		return p.allCache, nil
	}
	p.mu.RUnlock()

	fmt.Println("  [Cache] MISS FindAll，查询数据库...")
	users, err := p.real.FindAll()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.allCache = users
	p.mu.Unlock()
	return users, nil
}

func (p *CacheProxy) Save(user *User) error {
	err := p.real.Save(user)
	if err != nil {
		return err
	}
	// 写入后让缓存失效
	p.mu.Lock()
	delete(p.cache, user.ID)
	p.allCache = nil // 列表缓存也失效
	p.mu.Unlock()
	fmt.Printf("  [Cache] 缓存失效 id=%d\n", user.ID)
	return nil
}

// ----------------------------------------
// 代理二：权限代理
// ----------------------------------------

type Role string

const (
	RoleAdmin Role = "admin"
	RoleGuest Role = "guest"
)

type AuthProxy struct {
	real UserRepository
	role Role
}

func NewAuthProxy(real UserRepository, role Role) *AuthProxy {
	return &AuthProxy{real: real, role: role}
}

func (p *AuthProxy) FindByID(id int) (*User, error) {
	fmt.Printf("  [Auth] role=%s 请求 FindByID\n", p.role)
	return p.real.FindByID(id) // 所有角色可读
}

func (p *AuthProxy) FindAll() ([]*User, error) {
	fmt.Printf("  [Auth] role=%s 请求 FindAll\n", p.role)
	return p.real.FindAll()
}

func (p *AuthProxy) Save(user *User) error {
	fmt.Printf("  [Auth] role=%s 请求 Save\n", p.role)
	if p.role != RoleAdmin {
		return fmt.Errorf("权限不足：%s 无法执行写操作", p.role)
	}
	return p.real.Save(user)
}

// ----------------------------------------
// 辅助
// ----------------------------------------

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("-", 50))
}

// ========================================
// main
// ========================================

func main() {
	section("=== 代理一：缓存代理 ===")

	db := NewDBUserRepository("mysql://localhost:3306/prod")
	repo := NewCacheProxy(db)

	fmt.Println("\n-- 第一次查询 id=1（缓存 MISS）--")
	start := time.Now()
	u, _ := repo.FindByID(1)
	fmt.Printf("  结果: %+v 耗时=%v\n", *u, time.Since(start))

	fmt.Println("\n-- 第二次查询 id=1（缓存 HIT）--")
	start = time.Now()
	u, _ = repo.FindByID(1)
	fmt.Printf("  结果: %+v 耗时=%v\n", *u, time.Since(start))

	fmt.Println("\n-- 查询 id=2（缓存 MISS）--")
	u, _ = repo.FindByID(2)
	fmt.Printf("  结果: %+v\n", *u)

	fmt.Println("\n-- 查询 id=1 再次（缓存 HIT）--")
	u, _ = repo.FindByID(1)
	fmt.Printf("  结果: %+v\n", *u)

	fmt.Println("\n-- Save 触发缓存失效 --")
	repo.Save(&User{ID: 1, Name: "Alice-Updated", Age: 29})

	fmt.Println("\n-- 再次查询 id=1（缓存已失效，重新查 DB）--")
	u, _ = repo.FindByID(1)
	fmt.Printf("  结果: %+v\n", *u)

	fmt.Println("\n-- FindAll 两次（第二次命中缓存）--")
	repo.FindAll()
	repo.FindAll()

	section("=== 代理二：权限代理 ===")

	realDB := NewDBUserRepository("mysql://localhost:3306/prod")

	fmt.Println("\n-- Admin 角色 --")
	adminRepo := NewAuthProxy(realDB, RoleAdmin)
	adminRepo.FindByID(1)
	err := adminRepo.Save(&User{ID: 1, Name: "Alice", Age: 28})
	fmt.Println("  Save 结果:", err)

	fmt.Println("\n-- Guest 角色 --")
	guestRepo := NewAuthProxy(realDB, RoleGuest)
	guestRepo.FindByID(1)
	err = guestRepo.Save(&User{ID: 1, Name: "Alice", Age: 28})
	fmt.Println("  Save 结果:", err)

	section("=== 代理 vs 装饰器 的本质区别 ===")
	fmt.Println(`
  装饰器：调用方主动套上去，目的是增强功能
    handler = WithLogging(WithAuth(realHandler))
    ↑ 调用方清楚地知道在加日志和鉴权

  代理：调用方无感知，目的是控制访问
    var repo UserRepository = NewCacheProxy(db)
    ↑ 调用方只知道拿到了一个 UserRepository
      完全不知道内部有没有缓存、权限控制等`)
}
