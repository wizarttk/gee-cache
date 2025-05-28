// WARN:
// 此文件定义了分布式节点间服务调用的两大核心接口
// 用以解耦"选点"与"取值"的逻辑

package geecache

// PeerPicker接口 根据 key，选出负责该 key 的「远程对等节点 PeerGetter」（选点）
// 将"选点"逻辑抽象成接口，后续可灵活替换一致性哈希、简单轮询或其他策略
// - key: 要查找的缓存键
// 返回值
// - peer: 对应PeerGetter 客户端，用于从远程节点获取数据
//
// - ok: 是否选择了远程节点；若为 false，应回退本地获取逻辑
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter接口 定义了从远程缓存节点获取缓存值的方法抽象（取值）
// 各种网络客户端（如 httpGetter 或者 gRPC 客户端）需实现此接口，以便完成跨节点的数据访问
// 解耦网络传输细节，Group 只需调用此接口获取值，无需关心底层是 HTTP 还是 RPC
// - group: 命名空间，对应 Group 结构中的 name
// - key: 缓存键
// 返回值
// ([]byte, error)，返回缓存值字节切片或错误，以便上层封装 ByteView 和 错误处理逻辑
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
