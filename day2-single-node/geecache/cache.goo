// 这段代码实现了一个线程安全的 LRU 缓存系统，通过延迟初始化和只读数据封装来管理 ByteView 类型的数据。

// 当前代码（cache.go）基于 lru.Cache 实现了以下功能：
// 1. 线程安全支持（通过互斥锁确保并发访问安全）
// 2. 延迟初始化（首次添加数据时创建 LRU 缓存）（懒加载）（）
// 3. 特定类型（ByteView）的支持，确保数据只读性

package geecache

import (
	"geecache/lru"
	"sync"
)

// cache 结构体封装了 lru.Cache，并添加了线程安全支持及对 ByteView 类型的专门处理。
// 主要作用是管理缓存数据，确保并发访问时数据一致且安全。
type cache struct {
	mu         sync.Mutex // 互斥锁，确保对 lru 缓存的并发访问安全（因为add()和get()方法都涉及修改LRU缓存的顺序，本质上都是写操作，所以不能用读写锁）
	lru        *lru.Cache // 指向 lru 包中实现的 LRU 缓存的指针，存储实际的缓存数据
	cacheBytes int64      // 缓存的最大容量（字节为单位）
}

// add 方法用于向缓存中添加数据，具体流程如下：
// 1. 通过加锁确保线程安全操作。（确保多个goroutine并发写入缓存时不会发生数据竞争，特别是防止lru被重复初始化）
// 2. 如果 LRU 缓存尚未初始化，则根据 cacheBytes 延迟创建 LRU 缓存。
// 3. 将传入的键值对（其中 value 类型为 ByteView）存入 LRU 缓存。
func (c *cache) add(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 延迟初始化：只有在需要使用才创建 LRU 缓存实例
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value)
}

// get 方法用于从缓存中获取指定 key 的数据，具体说明如下：
// 1. 通过加锁确保线程安全。（防止多个 goroutine 并发访问 lru.Cache 时因修改 LRU 访问顺序导致数据竞争）
// 2. 如果 LRU 缓存尚未初始化，则直接返回 ByteView 的零值和 false。
// 3. 调用 LRU 缓存的 Get 方法查找数据，并使用类型断言将结果转换为 ByteView 类型。
// 4. 返回转换后的 ByteView 数据和表示查找成功与否的布尔值。
func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.lru == nil {
		return
	}

	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}

	return
}
