package lru

import (
	"reflect"
	"testing"
)

// String 类型实现了 Value 接口中的 Len 方法，返回字符串的长度
type String string

// Len 返回字符串所占用的字节数
func (d String) Len() int {
	return len(d)
}

// TestGet 测试 Get 方法，验证缓存中存在和不存在的情况
func TestGet(t *testing.T) {
	// 创建一个新缓存，最大内存设为 0（表示无限制），无回调函数
	lru := New(int64(0), nil)
	// 向缓存中添加键值对：key1 -> "1234"
	lru.Add("key1", String("1234"))

	// 测试：查找存在的 key1，预期返回 "1234"
	if v, ok := lru.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}

	// 测试：查找不存在的 key2，预期返回 false
	if _, ok := lru.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

// TestRemoveoldest 测试缓存淘汰策略，验证当超过内存限制时最老的记录被删除
func TestRemoveoldest(t *testing.T) {
	// 定义测试使用的三个键值对
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"

	// 计算内存容量：将 k1, k2, v1, v2 的长度相加作为最大内存
	cap := len(k1 + k2 + v1 + v2)
	// 创建缓存，设置的最大内存为 cap，无回调函数
	lru := New(int64(cap), nil)

	// 依次添加三个键值对
	lru.Add(k1, String(v1))
	lru.Add(k2, String(v2))
	lru.Add(k3, String(v3))

	// 此时，缓存容量已超出限制，因此 k1 应该被淘汰（最久未使用）
	// 测试：确保 key1 不在缓存中，且缓存中只剩下两个键值对
	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("Removeoldest key1 failed")
	}
}

// TestOnEvicted 测试回调函数 OnEvicted 是否正确调用
func TestOnEvicted(t *testing.T) {
	// 用于保存被淘汰键的切片
	keys := make([]string, 0)

	// 定义回调函数，当缓存淘汰某个条目时，将键加入 keys 切片中
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	// 创建缓存，设置最大内存为 10，并传入回调函数
	lru := New(int64(10), callback)

	// 添加多个键值对，触发淘汰过程
	lru.Add("key1", String("123456"))
	lru.Add("k2", String("k2"))
	lru.Add("k3", String("k3"))
	lru.Add("k4", String("k4"))

	// 预期淘汰的键为 "key1" 和 "k2"
	expect := []string{"key1", "k2"}

	// 使用 reflect.DeepEqual 比较回调中记录的淘汰键是否符合预期
	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s", expect)
	}
}

// TestAdd 测试 Add 方法，验证更新缓存中已有键时内存使用量是否正确计算
func TestAdd(t *testing.T) {
	// 创建一个无限制内存的缓存
	lru := New(int64(0), nil)
	// 首次添加键 "key" 对应值 "1"
	lru.Add("key", String("1"))
	// 更新键 "key"，新值为 "111"
	lru.Add("key", String("111"))

	// 计算预期内存：键 "key" 的长度加上新值 "111" 的长度
	expectedBytes := int64(len("key") + len("111"))
	// 如果计算的内存使用量不匹配，则测试失败
	if lru.nbytes != expectedBytes {
		t.Fatal("expected", expectedBytes, "but got", lru.nbytes)
	}
}
