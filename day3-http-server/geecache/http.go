// 提供被其他节点访问的能力（基于http）
package geecache

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// defaultBasePath 默认的 URL前缀,用于区分不同服务的请求
const defaultBasePath = "/_geecache/"

// HTTPPool结构体
// 该结构体同时充当 HTTP 服务的 Handler，处理来自其他节点的 HTTP 请求（服务端）
type HTTPPool struct {
	self     string // 当前节点的地址（主机名或IP和端口号）例："https://example.com:8000"
	basePath string // 节点间通信的统一路径前缀 例："/_geecache/"
	// 那么http://example.com:8000/_geecache/开头的请求，就用于节点间的访问
}

// NewHTTPPool 初始化一个 HTTP 节点池，并返回指向 HTTPPool 的指针
func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

// Log方法 用于统一输出日志信息，便于调试
func (p *HTTPPool) Log(format string, v ...interface{}) {
	// 在日志中打印当前处理请求的节点地址以及自定义消息
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

// ServeHTTP 实现了 http.Handler 接口，作为服务端，用于处理所有传入的 HTTP 请求
// WARN:
// 功能：
// 根据请求 URL 分析出缓存分组和键，
// 然后从对应分组获取缓存数据，再把数据作为 HTTP 响应返回；
// 如果出现问题，它会返回适当的错误信息。
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 判断请求路径（r.URL.Path）是否以我们预设的基础路径（p.basePath）开头
	// 只有以 basePath 开头的请求才由GeeCache 处理
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		// 如果不是，则说明请求不是针对缓存服务的，直接抛出错误
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}

	// 输出日志，记录请求的方法和路径
	p.Log("%s %s", r.Method, r.URL.Path)

	// 解析请求路径，要求格式为  /<basepath>/<groupname>/<key>
	// 通过将基础路径去掉后，再以"/"进行分割，最多分为2部分
	// 请求路径 r.URL.Path  如："/_geecache/score/Tom"
	// 基础路径 p.basePath  如："/_geecache/"
	// parts == [score, Tom]
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)

	// 如果分割后得到的部分数不等于2，说明请求路径格式错误
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// 分别获取缓存分组名称（groupName）和要查询的 key
	groupName := parts[0]
	key := parts[1]

	// 根据 groupName 找到对应的缓存分组
	group := GetGroup(groupName)
	if group == nil {
		// 如果未找到对应的组，返回 404 错误
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	// 从缓存分组中获取缓存数据
	view, err := group.Get(key)
	if err != nil {
		// 如果获取过程中出错，则返回 500 错误，并输出错误信息
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头，指定返回内容为二进制流
	w.Header().Set("Content-Type", "application/octet-stream")
	// 将获取到的缓存数据写入 HTTP 相应体中返回
	w.Write(view.ByteSlice())
}
