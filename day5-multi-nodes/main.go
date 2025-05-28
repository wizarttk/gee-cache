package main

/*
$ curl "http://localhost:9999/api?key=Tom"
630

$ curl "http://localhost:9999/api?key=kkk"
kkk not exist
*/

import (
	"flag"
	"fmt"
	"geecache"
	"log"
	"net/http"
)

// db 模拟了一个“慢速数据库”或外部存储。
// 缓存未命中时，程序会在这里查找并“回源”。
// map[string]string 保持简单，value 是字符串；回源时再转成 []byte。
var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// createGroup 创建缓存分组
// - 命名空间 "scores"：针对不同业务，可用多个 Group。
// - 容量 2<<10 (~2048 bytes)：LRU 缓存的上限，超过会淘汰最老数据。
// - 回调函数：通过 GetterFunc 适配器，把普通函数包装为 Getter，当本地/远程都无命中时调用：
//   - 1. 在控制台打印 [SlowDB] search key X，模拟慢查询。
//   - 2. 查 db，存在返回值字节，不存在返回错误。
func createGroup() *geecache.Group {
	return geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

// startCacheServer 启动缓存节点
// addr: 当前节点的地址（含协议和端口），如 "http://localhost:8001"
// addrs: 集群中所有节点的地址列表
// gee: 缓存分组实例
func startCacheServer(addr string, addrs []string, gee *geecache.Group) {
	// 1. 构造 HTTPPool，传入本节点地址
	peers := geecache.NewHTTPPool(addr)
	// 2. 将集群所有节点注册到一致性哈希环中
	peers.Set(addrs...)
	// 3. 将 HTTPPool 注入到 Group（依赖注入），启用分布式获取能力
	gee.RegisterPeers(peers)

	log.Println("geecache is running at", addr)
	// 4. 启动 HTTP 服务，监听 addr[7:] 端口（去掉前缀"http://"），所有路由交给 peers 处理
	//    peers.ServeHTTP 负责 /_geecache/<group>/<key> 路由
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}

// startAPIServer 启用前端HTTP服务，暴露 /api?key= 供外部客户端（例如 curl）通过 HTTP 接口访问 GeeCache。
// apiAddr: 前端监听地址（含协议和端口），如 "http://localhost:9999"
func startAPIServer(apiAddr string, gee *geecache.Group) {
	// 注册 /api 路由
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// 1. 从 URL 获取 key 参数
			key := r.URL.Query().Get("key")
			// 2. 调用分布式缓存获取数据（本地->远程->回源）
			view, err := gee.Get(key)
			if err != nil {
				// 4. 获取失败，返回 500 和错误信息
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// 3. 成功则以二进制形式返回数据
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())
		}))
	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func main() {
	// 解析命令行参数
	var port int                                             // 储存从命令行解析出来的端口号
	var api bool                                             // 存储从命令行解析出来的，是否启动 API 服务器的标志
	flag.IntVar(&port, "port", 8001, "Geecache server port") // 把命令行参数 -port 绑定到变量 port，默认值为 8001
	flag.BoolVar(&api, "api", false, "Start a api server?")  // 把命令行参数 -api 绑定到变量 api，默认值为 false
	flag.Parse()                                             // 启动解析过程，填充对应变量

	// 定义前端API地址
	apiAddr := "http://localhost:9999"
	// 定义集群中所有缓存节点的地址映射
	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	// 将映射中的地址收集到slice，用于一次性哈希注册
	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	// 1. 创建缓存分组
	gee := createGroup()

	// 2. 如果开启api标志，则在后台启动前端 API 服务
	//  监听 apiAddr[7:]（即 localhost:9999），
	//  注册 /api?key= 路由，把请求转给 gee.Get(key)，统一入口给客户端查询。
	if api {
		go startAPIServer(apiAddr, gee)
	}

	// 3. 启动缓存节点服务（阻塞调用）
	startCacheServer(addrMap[port], addrs, gee)
}
