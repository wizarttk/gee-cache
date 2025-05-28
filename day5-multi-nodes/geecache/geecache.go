// WARN:
// 改动
// Group的获取流程从「本地缓存 -> 本地回调」升级为「本地缓存 -> 远程节点 -> 本地回调」,
// 实现了分布式缓存功能

package geecache

import (
	"fmt"
	"log"
	"sync"
)

type Group struct {
	name      string
	getter    Getter
	mainCache cache
	// 依赖注入： 将一个对象所依赖的其他对象，通过外部的方式传递给它，而不是由它自己创建的方式，就是依赖注入。
	// 在 Group 结构体中使用 PeerPicker 接口作为字段，并通过 RegisterPeers 方法注入具体的 PeerPicker 实现，是依赖注入这一设计模式的典型应用，同时也遵循了面向接口编程的设计原则。
	peers PeerPicker // NEW: peers字段 分布式场景下的"选点"抽象接口，当Group发生缓存未命中时，他会调用peers的方法（例如 PickPeer(key string)），将key传入远程节点，通过该节点的代理对象（httpGetter）获取数据
}

// Getter 是用户回调接口：当本地和远程都未命中时，调用它从源头加载数据
type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
	}
	groups[name] = g
	return g
}

func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}

	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}

	return g.load(key)
}

// NEW:
// RegisterPeers 在分布式场景下为当前 Group 注入 PeerPicker 实例（HTTPPool），
// 在程序启动时，外部（main.go）会把实现了一致性哈希和 HTTP 客户端的 HTTPPool 注入进来，完成分布式路由组件的注册。
// 使得后续在 load 流程中能够通过 peers.PickPeer() 选取远程节点并发起请求。
func (g *Group) RegisterPeers(peers PeerPicker) {
	// 如果已经注册过一次，则再次注册属于逻辑错误，直接 panic 提醒开发者
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	// 将传入的 PeerPicker 保存到 Group.peers 字段中，
	// 之后 Group.load 会使用它来决定是否向远程节点获取数据
	g.peers = peers
}

// PERF:
// load 负责缓存未命中时的数据获取策略：
// 1. 如果注册了 g.peers（"选点"抽象接口），先通过 g.peers.PickPeer 选节点并尝试远程拉取
// 2. 远程失败或未注册 peers，回退到本地回调
func (g *Group) load(key string) (value ByteView, err error) {
	if g.peers != nil { // 如果注册了 PeerPicker（即处于分布式模式）
		// 通过一致性哈希选出负责该 key 的节点（PeerGetter）
		if peer, ok := g.peers.PickPeer(key); ok {
			// 如果选中了远程节点，就调用 getFromPeer 向它发起请求，取出缓存数据
			if value, err = g.getFromPeer(peer, key); err == nil {
				return value, nil // 直接返回数据
			}
			log.Println("[GeeCache] Failed to get from peer", err) // 远程拉取出错时，打印日志，继续回退到本地获取
		}
	}

	// 分布式模式未命中或未启用 peers，调用本地回调加载并缓存
	return g.getLocally(key)
}

func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

// 将从源头或远程获取的数据添加到本地缓存
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

// NEW:
// getFromPeer 通过 PeerGetter 接口从远程节点获取缓存数据
// 将 “网络字节” 转换为本地 ByteView 结构。
// - peer "代表远程节点客户端"  ---谁来取值
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	// 向远程 peer 发起 Get请求，参数是当前的 group 的 name（命名空间）和具体的 key
	// peer 在这里是*httpGetter，它知道怎样通过 HTTP 向某台缓存服务器（由 peer 标识）发起请求。
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		// 如果远程调用失败（网络、对段错误等）,将错误向上层返回
		return ByteView{}, err
	}
	// 成功拿到字节后，直接用这些字节构造一个只读的ByteView返回
	return ByteView{b: bytes}, nil
}
