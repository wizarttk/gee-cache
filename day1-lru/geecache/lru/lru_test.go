package lru

import (
	"reflect"
	"testing"
)

type String string

func (d String) Len() int {
	return len(d)
}

// 测试基本的存取操作
func TestGet(t *testing.T) {
	lru := New(int64(0), nil)       // 创建一个缓存（设置maxBytes为0表示不进行容量限制
	lru.Add("key1", String("1234")) // 添加键 "key1" 与对应值 String("1234")
	// 使用Get("key1")验证能正确获取刚添加的条目，同时检查 Get("key2")返回未命中
	if v, ok := lru.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	if _, ok := lru.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

// 测试淘汰策略
func TestRemoveoldest(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	cap := len(k1 + k2 + v1 + v2)
	lru := New(int64(cap), nil) // 设置一个有限的内存上限，该值通过两个key与对应value的长度之和计算
	// 先后添加爱三个键值对，当第三个添加后，缓存超出上限
	lru.Add(k1, String(v1))
	lru.Add(k2, String(v2))
	lru.Add(k3, String(v3))

	// 通过 Get("key1") 检查 "key1" 是否被淘汰
	// 通过 Len() 检查当前缓存中剩余条目的数量是否为
	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("Removeoldest key1 failed")
	}
}

// 测试淘汰回调
func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	// 创建一个缓存并设置 OnEvicted 回调，该回调将每次淘汰额key记录到一个slice中
	lru := New(int64(10), callback)   // 创建一个最大容量为10字节的缓存，并传入一个互调函数callback
	lru.Add("key1", String("123456")) // "key1"长度为4字节，值"123456"长度为6字节，总计占用10字节
	lru.Add("k2", String("k2"))       // 总共4字节，超过maxBytes，缓存调用RemoveOldest()
	lru.Add("k3", String("k3"))       // 总共4字节
	lru.Add("k4", String("k4"))       // 总共4字节，超过maxBytes，缓存再次调用RemoveOldest()
	expect := []string{"key1", "k2"}  // 期望的被淘汰的key切片

	// 使用 reflect.DeepEqual 检查实际淘汰顺序是否与预期一致
	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s", expect)
	}
}

// 测试更新操作
func TestAdd(t *testing.T) {
	lru := New(int64(0), nil)
	// 使用相同的key先后插入不同的值，检查更新后的内存占用是否正确。
	lru.Add("key", String("1"))
	lru.Add("key", String("111"))

	if lru.nbytes != int64(len("key")+len("111")) {
		t.Fatal("expected 6 but got", lru.nbytes)
	}
}
