/*
 在这个 GeeCache 的分布式场景中：

 - 谁是“客户端”？
   发起请求获取数据的那个节点，就是客户端。
 当一个 GeeCache 节点（比如 A）收到一个数据请求，但在自己的本地缓存中没有找到（Cache Miss）时，它需要去其他节点寻找数据。这时，节点 A 就变成了客户端，它要向拥有数据的那个节点发起请求。

 - 谁是“服务端”？
   接收请求并提供数据的那个节点，就是服务端。
 一致性哈希算法告诉节点 A，数据可能在节点 B 上。节点 B 运行着一个 HTTP 服务，专门用于响应其他节点的数据请求。这时，节点 B 就扮演了服务端的角色，它等待来自其他节点（客户端）的请求。
-------------
HTTPPool 和 httpGetter 的关系：
  - HTTPPool 是一个Peer 管理者和请求分发者。它知道集群中有哪些 Peer，并且能够根据 Key 选择合适的 Peer。它也知道如何与每个 Peer 通信（通过对应的 httpGetter）。
  - httpGetter 是一个具体的 HTTP 客户端实现。每一个 httpGetter 实例都绑定到一个特定的远程 Peer，负责向那个 Peer 发起实际的 HTTP 请求。
所以，HTTPPool 本身不直接发起所有的客户端请求，它更像是一个工厂和管理器，管理着与每个 Peer 通信所需的 httpGetter 客户端实例。
当需要作为客户端获取数据时，HTTPPool 负责选择正确的“信使”（httpGetter），然后让那个“信使”去执行具体的 HTTP 请求。
*/
// WARN:
// 改动
// Day5 的改动让缓存层能基于一致性哈希在多台节点间自动路由和负载均衡，
// 实现了分布式缓存的可扩展性和容错性。
package geecache

import (
	"fmt"
	"geecache/consistenthash"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultBasePath = "/_geecache/"
	defaultReplicas = 50 // NEW: 哈希环中每个真实节点对应的虚拟节点的倍数
)

// HTTPPool 的核心职责有：
// 1. 节点路由：基于一致性哈希维护 peers 和 httpGetters，实现 PeerPicker 接口，根据 key 选出目标节点；
// 2. 远程调用：通过 httpGetter 对选中的远程节点发起 HTTP 请求，获取缓存数据；
// 3. HTTP 服务：实现 http.Handler，对外暴露 /_geecache/<group>/<key> 接口，处理其它节点或客户端的缓存请求。
type HTTPPool struct {
	self        string
	basePath    string
	mu          sync.Mutex             // NEW: 保护 peer 和 httpGetters 并发访问（Set 和 PickPeer 会并发读写 peers 和 httpGetters，需要锁来保证操作的原子性及可见性，避免竞态条件。）
	peers       *consistenthash.Map    // NEW: 一致性哈希环,用于根据 key 选节点
	httpGetters map[string]*httpGetter // NEW: 映射,(远程节点地址 -> 对应客户端httpGetter )。每一个远程节点对应一个 httpGetter，因为 httpGetter 与远程节点的地址 baseURL 有关。
}

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	p.Log("%s %s", r.Method, r.URL.Path)

	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())
}

// NEW:
// Set 接收一组"远程节点地址 peers"，构造一致性哈希环（p.peers）
// 并为每个节点初始化 httpGetter 客户端
//   - peers 要加入到缓存集群中的真实节点的地址列表
//     写操作
func (p *HTTPPool) Set(peers ...string) {
	// 1. 构造空的哈希环
	p.peers = consistenthash.New(defaultReplicas, nil)
	// 2. 将传入的真实节点地址添加到哈希环中（内部会为每个地址生成多个虚拟节点并排序）
	p.peers.Add(peers...)

	// 3. 为后续远程调用准备 map：键是节点地址，值是该节点的 httpGetter 客户端
	p.httpGetters = make(map[string]*httpGetter, len(peers))

	// 3. 遍历所有真实节点地址，为每个地址构造一个httpGetter客户端
	//     httpGetter.baseURL = 节点地址peer + basePath
	//     用于后续向其他远程节点发起缓存请求：baseURL/group/key
	for _, peer := range peers {
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

// NEW:
// PickPeer 根据 key 做一致性哈希，选出负责该key的节点。
// 如果没有选出远程节点，或选中自己，则返回(nil,false)
//   - key 要查找的缓存键
//     读操作
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 一致性哈希选点：根据 key 的哈希值，在 p.peers（一致性哈希环）上顺时针找到第一个虚拟节点
	// 再映射回真实节点地址 peer（string）。
	// 判断选出的 peer 是否有效且不是自己：
	//		- peer == "" 哈希环上无节点
	//		- peer == p.self 说明key落在自己负责的区间，不应向远程请求
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		// 记录日志，便于调试：表明此 key 被路由到远程节点 peer
		p.Log("Pick peer %s", peer)
		// 返回该 peer 对应的 HTTP 客户端（实现 PeerGetter），以及 true 标志
		return p.httpGetters[peer], true
	}
	// 2. 如果没有选出远程节点，或选中自己，则返回(nil,false)
	//		上层会检测到 false 并回退到本地处理逻辑
	return nil, false
}

// NEW:
// 编译期断言：HTTPPool 必须实现 PeerPicker 接口
// 将一个类型为 *HTTPPool 的“零值指针”赋给接口类型 PeerPicker 的匿名变量 _。
// 如果 *HTTPPool 没有实现 PeerPicker 接口的所有方法（即 PickPeer(string) (PeerGetter, bool)），编译器会报错。
// 因为赋值目标是空白标识符 _，运行时并不会真的存储该值——它仅在编译时生效。
//
// 为什么要这么写？
// 1. 提早报错
// 把接口契约的校验前置到编译阶段，而不是等到运行时才因方法缺失而崩溃。
//
// 2. 文档与自检
// 一眼就能看到哪些类型承担了哪些接口角色，相当于在代码里写下 “我保证 HTTPPool 是 PeerPicker” 的声明。
//
// 3. 零开销
// 因为赋给了空白标识符 _，不会产生任何运行时开销或存储。
var _ PeerPicker = (*HTTPPool)(nil)

// NEW: HTTP客户端，负责对远程节点发起请求
// httpGetter 实现了 PeerGetter 接口，负责通过 HTTP 向特定远程节点获取缓存
type httpGetter struct {
	// 1. baseURL 储存的就是一个远程 GeeCache 节点的 HTTP 服务的基础 URL，
	// 		形如："http://http://localhost:8001/_geecache/" （节点地址 + bashPath）
	// 2. 当需要从这个远程节点获取某个 group 下的某个 key 的数据时，
	//		httpGetter 会使用这个 baseURL，并拼接上 group 名称和 key，
	//		构造出完整的请求 URL: http://localhost:8001/geecache/<group>/<key>），
	//		然后向这个 URL 发起 HTTP GET 请求。
	baseURL string // 不同的远程节点的 baseURL 的区别在于它们指向了不同的网络地址和端口
}

// NEW:
// Get方法 客户端向远程节点发起 HTTP 请求，获取特定缓存组中某个键对应的值（实现了PeerGetter 接口）（发起请求/读取相应体）
// 1. URL 组装：h.baseURL 已含 /_geecache/ 前缀，随后拼接转义后的 group 和 key，形成完整路径。
// 2. 发起请求：直接调用 http.Get(u)。若网络或地址错误，立即失败返回。
// 3. 状态校验：仅在远程返回 HTTP 200 时继续；否则将状态码封装为错误。
// 4. 读取响应：用 io.ReadAll 获取所有响应体字节，并返回给上层。
func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	// 1. 构造请求 URL (h.baseURL 已含 /_geecache/ 前缀，随后拼接转义后的 group 和 key，形成完整路径。)
	//    h.baseURL 形如 "http://<peerAddr>/_geecache/"
	//    对 group 和 key 做 URL 转义，防止特殊字符破坏路径
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group),
		url.QueryEscape(key),
	)

	// 2. 发起 HTTP GET 请求
	//    这里使用 Go 标准库 http.Get，简洁但无连接复用优化
	res, err := http.Get(u)
	if err != nil {
		// 网络错误或无法连接时直接返回
		return nil, err
	}
	// 确保在函数返回前关闭响应体，防止连接泄漏
	defer res.Body.Close()

	// 3. 检查 HTTP 状态码，非 200 视为失败
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	// 4. 读取全部响应数据
	//    返回的应该是服务端写回的 value 字节流
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}

	// 5. 成功则返回字节切片
	return bytes, nil
}

// NEW: 编译期断言：httpGetter 必须实现 PeerGetter 接口
var _ PeerGetter = (*httpGetter)(nil)
