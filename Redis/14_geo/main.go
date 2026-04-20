// 案例14：Redis GEO —— 地理位置
// 知识点：GEOADD / GEOPOS / GEODIST / GEOSEARCH
//
// Redis GEO 底层用 ZSet 存储，经纬度编码成 score
// 典型场景：
//
//	① 附近的人/店铺
//	② 骑手/司机位置追踪
//	③ 配送范围判断
//	④ 打车距离计算
package main

import (
	"context"
	"fmt"
	"math"

	"github.com/redis/go-redis/v9"
)

// 餐厅结构
type Restaurant struct {
	Name      string
	Longitude float64
	Latitude  float64
	Category  string
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: GEOADD / GEOPOS 添加和查询位置 ===")
	case1_addPos(ctx, rdb)

	fmt.Println("\n=== case2: GEODIST 计算两点距离 ===")
	case2_dist(ctx, rdb)

	fmt.Println("\n=== case3: GEOSEARCH 搜索附近的餐厅 ===")
	case3_nearby(ctx, rdb)

	fmt.Println("\n=== case4: 综合案例 —— 附近的骑手 ===")
	case4_nearbyRider(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 添加和查询位置
// ─────────────────────────────────────────────
func case1_addPos(ctx context.Context, rdb *redis.Client) {
	key := "geo:cities"
	rdb.Del(ctx, key)

	// GEOADD key longitude latitude member
	// 注意：是先经度后纬度
	cities := []*redis.GeoLocation{
		{Name: "北京", Longitude: 116.4074, Latitude: 39.9042},
		{Name: "上海", Longitude: 121.4737, Latitude: 31.2304},
		{Name: "广州", Longitude: 113.2644, Latitude: 23.1291},
		{Name: "深圳", Longitude: 114.0579, Latitude: 22.5431},
		{Name: "杭州", Longitude: 120.1551, Latitude: 30.2741},
	}

	added, _ := rdb.GeoAdd(ctx, key, cities...).Result()
	fmt.Printf("添加了 %d 个城市\n", added)

	// GEOPOS 查询指定成员的经纬度
	positions, _ := rdb.GeoPos(ctx, key, "北京", "上海", "不存在的城市").Result()
	names := []string{"北京", "上海", "不存在的城市"}
	for i, pos := range positions {
		if pos == nil {
			fmt.Printf("  %s → 不存在\n", names[i])
		} else {
			fmt.Printf("  %s → 经度:%.4f 纬度:%.4f\n", names[i], pos.Longitude, pos.Latitude)
		}
	}

	rdb.Del(ctx, key)
}

// ─────────────────────────────────────────────
// case2: 计算两点距离
// ─────────────────────────────────────────────
func case2_dist(ctx context.Context, rdb *redis.Client) {
	key := "geo:cities"
	rdb.Del(ctx, key)

	rdb.GeoAdd(ctx, key,
		&redis.GeoLocation{Name: "北京", Longitude: 116.4074, Latitude: 39.9042},
		&redis.GeoLocation{Name: "上海", Longitude: 121.4737, Latitude: 31.2304},
		&redis.GeoLocation{Name: "广州", Longitude: 113.2644, Latitude: 23.1291},
		&redis.GeoLocation{Name: "深圳", Longitude: 114.0579, Latitude: 22.5431},
	)

	// GEODIST key member1 member2 [unit]
	// unit: m(米) km(千米) mi(英里) ft(英尺)
	pairs := [][]string{
		{"北京", "上海"},
		{"上海", "广州"},
		{"广州", "深圳"},
	}

	for _, pair := range pairs {
		dist, _ := rdb.GeoDist(ctx, key, pair[0], pair[1], "km").Result()
		fmt.Printf("  %s → %s: %.1f km\n", pair[0], pair[1], dist)
	}

	rdb.Del(ctx, key)
}

// ─────────────────────────────────────────────
// case3: GEOSEARCH 搜索附近的餐厅
// ─────────────────────────────────────────────
func case3_nearby(ctx context.Context, rdb *redis.Client) {
	key := "geo:restaurants"
	rdb.Del(ctx, key)

	// 添加餐厅数据（以上海某区域为中心）
	restaurants := []*redis.GeoLocation{
		{Name: "麦当劳-人民广场店", Longitude: 121.4737, Latitude: 31.2304},
		{Name: "肯德基-南京路店", Longitude: 121.4790, Latitude: 31.2350},
		{Name: "星巴克-外滩店", Longitude: 121.4900, Latitude: 31.2400},
		{Name: "海底捞-静安寺店", Longitude: 121.4480, Latitude: 31.2240},
		{Name: "喜茶-陆家嘴店", Longitude: 121.5010, Latitude: 31.2390},
	}
	rdb.GeoAdd(ctx, key, restaurants...)

	// 用户当前位置（人民广场附近）
	userLon, userLat := 121.4737, 31.2304

	// GEOSEARCH：搜索指定范围内的成员
	result, _ := rdb.GeoSearch(ctx, key, &redis.GeoSearchQuery{
		Longitude:  userLon,
		Latitude:   userLat,
		Radius:     3,     // 半径
		RadiusUnit: "km",  // 单位
		Sort:       "ASC", // 按距离升序
		Count:      5,     // 最多返回5个
	}).Result()

	fmt.Printf("用户位置(%.4f, %.4f) 3km内的餐厅:\n", userLon, userLat)
	for _, name := range result {
		// 单独查距离
		dist, _ := rdb.GeoDist(ctx, key, "麦当劳-人民广场店", name, "km").Result()
		fmt.Printf("  %s (%.2fkm)\n", name, dist)
	}

	rdb.Del(ctx, key)
}

// ─────────────────────────────────────────────
// case4: 综合案例 —— 附近的骑手
// 模拟：用户下单，系统找附近3km内的空闲骑手
// ─────────────────────────────────────────────

type Rider struct {
	ID        string
	Name      string
	Longitude float64
	Latitude  float64
	Status    string // idle:空闲 busy:配送中
}

func case4_nearbyRider(ctx context.Context, rdb *redis.Client) {
	geoKey := "riders:location"
	statusKey := "riders:status"
	rdb.Del(ctx, geoKey, statusKey)

	// 骑手数据
	riders := []Rider{
		{ID: "r001", Name: "张三", Longitude: 121.4800, Latitude: 31.2320, Status: "idle"},
		{ID: "r002", Name: "李四", Longitude: 121.4600, Latitude: 31.2280, Status: "busy"},
		{ID: "r003", Name: "王五", Longitude: 121.4750, Latitude: 31.2350, Status: "idle"},
		{ID: "r004", Name: "赵六", Longitude: 121.5100, Latitude: 31.2500, Status: "idle"},
		{ID: "r005", Name: "钱七", Longitude: 121.4650, Latitude: 31.2310, Status: "busy"},
	}

	// 批量写入骑手位置和状态
	_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, r := range riders {
			pipe.GeoAdd(ctx, geoKey, &redis.GeoLocation{
				Name:      r.ID,
				Longitude: r.Longitude,
				Latitude:  r.Latitude,
			})
			pipe.HSet(ctx, statusKey, r.ID, r.Status)
		}
		return nil
	})
	if err != nil {
		fmt.Println("写入失败:", err)
		return
	}

	// 用户下单位置
	orderLon, orderLat := 121.4737, 31.2304
	fmt.Printf("订单位置: (%.4f, %.4f)\n", orderLon, orderLat)
	fmt.Println("搜索3km内的骑手:")

	// 搜索附近骑手（带距离）
	nearby, _ := rdb.GeoSearchLocation(ctx, geoKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  orderLon,
			Latitude:   orderLat,
			Radius:     3,
			RadiusUnit: "km",
			Sort:       "ASC",
		},
		WithDist: true,
	}).Result()

	// 过滤空闲骑手
	fmt.Println("附近骑手列表:")
	var idleRiders []redis.GeoLocation
	for _, loc := range nearby {
		status, _ := rdb.HGet(ctx, statusKey, loc.Name).Result()

		// 找到骑手名字
		riderName := ""
		for _, r := range riders {
			if r.ID == loc.Name {
				riderName = r.Name
				break
			}
		}

		statusStr := "🟢 空闲"
		if status == "busy" {
			statusStr = "🔴 配送中"
		}
		fmt.Printf("  骑手%s(%s) 距离:%.2fkm %s\n",
			riderName, loc.Name, loc.Dist, statusStr)

		if status == "idle" {
			idleRiders = append(idleRiders, loc)
		}
	}

	// 派单给最近的空闲骑手
	if len(idleRiders) > 0 {
		best := idleRiders[0]
		riderName := ""
		for _, r := range riders {
			if r.ID == best.Name {
				riderName = r.Name
				break
			}
		}
		fmt.Printf("\n🚴 派单给: %s(%s) 距离:%.2fkm\n",
			riderName, best.Name, best.Dist)
		// 更新骑手状态为配送中
		rdb.HSet(ctx, statusKey, best.Name, "busy")
	}

	// 验证距离计算（用 Haversine 公式验证）
	fmt.Println("\n距离验证（Redis vs Haversine公式）:")
	for _, r := range riders[:2] {
		dist, _ := rdb.GeoDist(ctx, geoKey, "r001", r.ID, "km").Result()
		hDist := haversine(riders[0].Latitude, riders[0].Longitude, r.Latitude, r.Longitude)
		fmt.Printf("  r001→%s: Redis=%.3fkm Haversine=%.3fkm\n", r.ID, dist, hDist)
	}

	rdb.Del(ctx, geoKey, statusKey)
}

// Haversine 公式计算两点距离（km）
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
