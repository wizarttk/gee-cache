// 这段代码演示了如何利用 GeeCache 模块搭建一个简单的缓存服务，并通过 HTTP 接口提供外部访问的能力。
// 当请求到来时，如果缓存中没有数据，会通过回调函数（模拟数据源）进行查找，并将结果返回给客户端。
package main

import (
	"fmt"
	"geecache"
	"log"
	"net/http"
)

// db 模拟的数据源，用 map 模拟数据源，其中存储了几个键值对
var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func main() {
	// 创建一个名为 scores 的缓存组，缓存上限设为 2<<10（即 2048）
	// 如果缓存未命中，将调用 GetterFunc（定义的匿名函数）从数据源查找数据
	geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			// 打印日志，表明此处正在通过数据源查找 key 对应的数据
			log.Println("[SlowDB] search key", key)
			// 从模拟数据库 db 中查找制定的 key
			if v, ok := db[key]; ok {
				// 如果存在则返回对应的值，并转换成字节数据
				return []byte(v), nil
			}
			// 如果未找到对应的 key，则返回错误提示
			return nil, fmt.Errorf("%s not exist", key)
		}))

	addr := "localhost:9999"
	// 创建一个 HTTPPool 实例，作为当前节点的 HTTP 服务端，负责处理 HTTP 请求
	peers := geecache.NewHTTPPool(addr)
	log.Println("geecache is running at", addr)
	// 启动 HTTP 服务
	// ListenAndServe 启动监听指定地址（addr）的服务器，并将 peers 作为请求的 Handler 处理所有请求
	// 使用 log.Fatal 如果服务器启动失败，则会输出错误日志并终止程序运行
	log.Fatal(http.ListenAndServe(addr, peers))
}
