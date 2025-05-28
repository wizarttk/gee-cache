package geecache

import (
	"fmt"
	"log"
	"reflect"
	"testing"
)

// 模拟数据库，用一个 map 保存用户的分数信息
var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// TestGetter 测试 Getter 接口的基本功能
func TestGetter(t *testing.T) {
	// 定义一个 Getter 类型 f，通过 GetterFunc 将匿名函数转换为 Getter 接口实现
	var f Getter = GetterFunc(func(key string) ([]byte, error) {
		// 这里简单返回 key 的字节数组作为结果，不做其他处理
		return []byte(key), nil
	})

	// 预期返回值为 "key" 的字节数组
	expect := []byte("key")
	// 调用 f.Get("key")，比较返回值与预期是否一致
	if v, _ := f.Get("key"); !reflect.DeepEqual(v, expect) {
		t.Fatal("callback failed")
	}
}

// TestGet 测试整个缓存组的功能，包括数据加载和缓存命中情况
func TestGet(t *testing.T) {
	// 记录每个 key 从数据库中加载的次数，用于验证缓存命中效果
	loadCounts := make(map[string]int, len(db))

	// 创建一个新的缓存组，名称为 "scores"，最大缓存容量为 2<<10（2048字节）
	// 同时传入一个 GetterFunc，用于从模拟数据库中加载数据
	gee := NewGroup("scores", 2<<10, GetterFunc(
		func(key string) ([]byte, error) {
			// 模拟从数据库中查找 key
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				// 记录每个 key 的加载次数
				if _, ok := loadCounts[key]; !ok {
					loadCounts[key] = 0
				}
				loadCounts[key]++
				// 返回对应的数据库值，转换为字节数组
				return []byte(v), nil
			}
			// 如果 key 不存在，返回错误
			return nil, fmt.Errorf("%s not exist", key)
		}))

	// 遍历模拟数据库中的所有键值对，验证缓存是否正确工作
	for k, v := range db {
		// 第一次调用 Get 应该从数据库加载数据
		if view, err := gee.Get(k); err != nil || view.String() != v {
			t.Fatal("failed to get value of Tom")
		}
		// 再次调用 Get 时，缓存应该命中，不会再次调用 GetterFunc
		if _, err := gee.Get(k); err != nil || loadCounts[k] > 1 {
			t.Fatalf("cache %s miss", k)
		}
	}

	// 测试一个不存在的 key，预期返回错误
	if view, err := gee.Get("unknown"); err == nil {
		t.Fatalf("the value of unknow should be empty, but %s got", view)
	}
}

// TestGetGroup 测试缓存组的管理功能，包括创建和获取缓存组
func TestGetGroup(t *testing.T) {
	groupName := "scores"

	// 创建一个新的缓存组，名称为 groupName
	NewGroup(groupName, 2<<10, GetterFunc(
		func(key string) (bytes []byte, err error) { return }))

	// 通过 GetGroup 获取已创建的缓存组，验证组名称是否正确
	if group := GetGroup(groupName); group == nil || group.name != groupName {
		t.Fatalf("group %s not exist", groupName)
	}

	// 尝试获取一个不存在的缓存组，预期返回 nil
	if group := GetGroup(groupName + "111"); group != nil {
		t.Fatalf("expect nil, but %s got", group.name)
	}
}
