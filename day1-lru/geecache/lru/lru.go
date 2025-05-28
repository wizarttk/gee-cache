package lru

import (
	"container/list" // 这个包实现了一个双向链表
)

// FIFO先进先出  LFU最少使用  LRU最近最少使用
// 如果数据最近被访问过，那么将来被访问的概率也会更高

// 核心数据结构
// - 使用 双向链表 和 哈希表 的组合实现缓存
// - 双向链表记录缓存项的访问顺序，哈希表用于快速定位缓存项。

// 缓存操作
// - 新数据插入或访问现有数据时，将对应的链表节点移动到链表的首部
// - 如果缓存总大小超过 maxBytes，则淘汰链表队尾的节点（最近最少使用）

// 淘汰策略
// - 当缓存项被淘汰时，触发回调函数 OnEvicted，用户可自定义逻辑

// 接口抽象
// - 值类型实现了 Value 接口，因此缓存可以存储任意类型的值，只要支持计算字节大小

// Cache 实现了 LRU（Least Recently Used）缓存，**非并发安全**
type Cache struct {
	maxBytes  int64                         // 允许的最大缓存容量（字节），超过该值时触发淘汰策略
	nbytes    int64                         // 当前已使用的缓存大小（字节）
	ll        *list.List                    // 双向链表，记录缓存项的访问顺序（链表头部表示最近使用的项）
	cache     map[string]*list.Element      // 哈希表，将字符串键映射到链表中的元素，从而实现 O(1) 的查找缓存项对应的*list.Element的操作。
	OnEvicted func(key string, value Value) // 可选的回调函数（可为 nil），在缓存项被移除时触发
}

// entry 代表缓存中的数据项（链表节点的值）
type entry struct {
	key   string // 对应缓存项的 key（在链表中仍保存每个值对应的 key 的好处在于，淘汰队首节点时，需要用 key 从字典中删除对应的映射。）
	value Value  // 缓存值，必须实现 Value 接口
}

// Value 接口定义了缓存值需要实现的方法
// - 该接口用于计算缓存项的内存占用，便于触发 LRU 淘汰策略
type Value interface {
	Len() int // 返回缓存值占用的内存大小
}

// New 创建并初始化一个 LRU 缓存实例
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// Add 方法：向 LRU 缓存中添加或更新键值对
// - 如果键已存在：更新该节点的值，并将其移动到队头（表示最近访问）
// - 如果键不存在：创建新节点（*list.Element），添加到队头，并在字典中添加 key 和 节点 的映射关系
// - 更新 c.nbytes（当前缓存占用字节数），如果超过 c.maxBytes（最大容量），则移除最久未使用的节点
func (c *Cache) Add(key string, value Value) {
	if ele, ok := c.cache[key]; ok { // 在哈希表中查找 key 是否已存在
		c.ll.MoveToFront(ele)                                  // 将该元素移动到链表头部（最近使用）
		kv := ele.Value.(*entry)                               // 类型断言，获取 entry 结构体
		c.nbytes += int64(value.Len()) - int64(kv.value.Len()) // 调整缓存的总字节数
		kv.value = value                                       // 更新 entry 的值
	} else { // key 不存在，添加新节点
		ele := c.ll.PushFront(&entry{key, value})        // 创建新 entry 并插入链表头部，获取新创建的链表元素ele
		c.cache[key] = ele                               // 在哈希表中建立 key 和链表节点的映射
		c.nbytes += int64(len(key)) + int64(value.Len()) // 更新缓存的总字节数
	}
	// 如果设置了内存限制，且超过该限制时，移除最久未使用的节点
	for c.maxBytes != 0 && c.maxBytes < c.nbytes { // c.maxBytes == 0 表示不对内存大小设限，所以不为0的时才会判断是否超过了限制
		c.RemoveOldest()
	}
}

// Get 从缓存中获取指定 key 对应的值
// - 如果 key 存在，则将对应节点移动到链表头部（表示最近访问），并返回其值
// - 如果 key 不存在，则返回 nil 和 false
func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele, ok := c.cache[key]; ok { // 在哈希表中查找 key 是否存在
		c.ll.MoveToFront(ele) // 访问后，将节点移动到链表头部（LRU 规则）
		// list.Element.Value 的类型是 interface{}，且在Add方法添加的时候，存储的是&entry{}，需要转换为 *entry 以访问存储的值
		kv := ele.Value.(*entry)
		return kv.value, true // 命中缓存，返回 value 和 true
	}
	return // 未命中缓存，返回 nil 和 false
}

// RemoveOldest 移除最近最少使用（LRU）的缓存项（缓存淘汰）
// - 具体表现为：删除链表尾部的节点（即最久未使用的数据）
// - 同时从哈希表中删除对应的映射，并更新内存使用量
// - 若提供了 OnEvicted 回调函数，则在删除后执行回调
func (c *Cache) RemoveOldest() {
	ele := c.ll.Back() // 获取链表的最后一个元素（最久未使用的数据）
	if ele != nil {    // 确保链表不为空，避免 nil 访问错误
		c.ll.Remove(ele)                                       // 从链表中删除该节点
		kv := ele.Value.(*entry)                               // 类型断言，将 interface{} 转换回 *entry
		delete(c.cache, kv.key)                                // 从哈希表中删除对应的 key
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len()) // 更新当前缓存的总字节数

		// 如果提供了 OnEvicted 回调函数，则调用回调，通知外部已删除的 key-value
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

// Len 返回缓存中当前存储的项数，等同于链表的长度
func (c *Cache) Len() int {
	return c.ll.Len()
}
