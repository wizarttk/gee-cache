// 负责与外部交互，控制缓存存储和获取的主流程
package geecache

import (
	"fmt"
	"log"
	"sync"
)

// 接收请求 key
// 检查是否被缓存 ：
// 1.缓存命中，直接返回缓存。
// 2.缓存未命中（决定从何处获取所需数据）：
//   2.1：从远程节点获取 --> 与远程节点交互，返回缓存值，并将数据添加到缓存中。
//   2.2: 本地获取 --> 调用已设定的回调函数在本地获取数据，获取后返回，并将数据添加到缓存中。

// Group 缓存组（命名空间），负责管理一个独立的缓存实例以及数据加载逻辑
type Group struct {
	name string // 缓存组的名称，用于唯一标识一个缓存命名空间，区分不同的缓存实例，实现数据隔离
	// 不应该支持多种数据源的配置：1. 数据源的种类太多 2. 拓展性不好 --> 用户可以根据自己的需求实现按一个回调函数。在缓存不存在时，调用这个函数，得到源数据。
	getter    Getter // 回调接口：缓存未命中时，通过用户提供的回调函数（实现 Getter 接口）从数据源（如数据库、文件等）获取数据。
	mainCache cache  // 内部缓存对象，封装了并发安全的 LRU 缓存逻辑
}

// Getter 接口定义了从数据源获取数据的行为（允许用户自定义数据获取方式）
type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc 接口型函数
// 函数类型实现某一个接口，称之为接口型函数，
// 这样允许用户直接传入一个函数作为 Getter 回调，而不必定义新的结构体。
type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

// 全局变量 用于管理所有创建的缓存组（Group）实例。
// - 全局存储保证了在整个程序生命周期内都能访问到这些 Group 实例。
var (
	mu     sync.RWMutex              // mu:     使用读写锁来保护对全局变量 groups 的并发访问。
	groups = make(map[string]*Group) // groups: 全局 map，储存和管理所有创建的 Group，使得在程序的其他部分可以通过Group 名称快速获取都应的缓存实例（Group名称指向Group实例的指针）。
)

// NewGroup 新建一个缓存组 Group 实例
func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	// 如果没有提供有效的 getter 回调函数，则直接panic，确保 Group 实例总能正确加载数据
	if getter == nil {
		panic("nil Getter")
	}

	mu.Lock()         // 对全局变量gorups进行写操作前，加上写锁来保证线程安全。
	defer mu.Unlock() // 使用defer保证在函数退出前一定会解锁，即使中间发生错误。

	// 创建一个新的 Group实例
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
	}

	// 写操作，将新创建的 Group 存入全局的 groups map中，以便后续通过名称获取
	groups[name] = g
	return g // 返回新创建的Group实例，供调用者后续使用。
}

// GetGroup 根据名称返回已创建的缓存组实例
// 这样可以实现对缓存组的集中管理和全局访问。
func GetGroup(name string) *Group {
	mu.RLock()        // 加读锁，确保在读取全局 groups map 中不会发生数据竞态。
	g := groups[name] // 根据 Group 名称从全局 groups map 中获取对应的 Group 实例。
	mu.RUnlock()      // 释放读锁
	return g          // 返回获取到的 Group 实例，如果不存在在返回 nil。
}

// Get方法 用于获取缓存中key对应的值
// 1. 参数校验
// 2. 本地缓存查找
// 3. 缓存未命中时加载数据
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" { // 如果 key 为空，则返回错误
		// 返回一个空的ByteView对象，表示没有有效的数据可以返回；生成一个error对象，其错误信息为 "key is required"，告诉调用者必须提供一个非空的 key。
		return ByteView{}, fmt.Errorf("key is required")
	}

	// 先尝试从本地缓存 mainCache 中查找数据
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit") // 如果「命中」则打印日志 [Geecache] hit
		return v, nil                 // 并立即返回缓存中的数据。
	}

	// 缓存「未命中」时调用 g.load(key) 加载数据
	return g.load(key)
}

// load方法 负责在缓存未命中时加载数据，
// - 当前实现中直接调用 getLocally 方法。从本地数据源（通过用户指定的Getter回调函数）获取数据
// - 该方法目前只是一个简单的转发函数，直接将请求交给 getLocally 处理。这种设计为后续拓展留出了接口。（未来可拓展，比如先从远程节点获取数据）
func (g *Group) load(key string) (value ByteView, err error) {
	return g.getLocally(key)
}

// getLocally方法 调用用户回调 Getter，从源获取数据并写入本地缓存
// 1. 从数据源获取数据，将原始数据克隆并封装为不可变的 ByteView，
// 2. 填充到本地缓存，并返回封装后的数据及错误信息。
func (g *Group) getLocally(key string) (ByteView, error) {
	// 1. 调用 Getter 回调函数从数据源（例如数据库、文件等）获取数据
	bytes, err := g.getter.Get(key)
	if err != nil {
		// 如果数据加载失败，返回空的 ByteView 和错误信息
		return ByteView{}, err
	}
	// 2. 克隆获取到的字节数据，确保数据不会被外部修改，
	//    封装为不可变的 ByteView 对象
	value := ByteView{b: cloneBytes(bytes)}
	// 3. 将加载到的数据填充到本地缓存中，便于后续对相同 key 的请求直接命中缓存
	g.populateCache(key, value)
	// 4. 返回封装后的 ByteView 对象和 nil 错误，表示加载成功
	return value, nil
}

// populateCache方法 将从数据源获取到的数据存入本地缓存（mainCache）
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value) // 因为 add 方法已经对缓存的并发访问进行了控制，因此 populateCache 不需要额外的并发处理。
}
