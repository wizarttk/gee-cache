/*
一致性哈希:
解决了在分布式系统中，当节点（服务器或缓存机器）数量发生变化（增加或减少）时，如何最小化数据或请求的重新分布，从而提高系统的可用性和伸缩性。

 1. 构建哈希环
    想象一个圆环，圆环上有一些数字，通常是一个很大的范围（比如0 到 2^32 - 1）。

 2. 映射「节点」到环上
    我们不再使用节点数量取模，而是先用一个哈希函数计算每个节点的哈希值，然后把这些节点映射到哈希换的不同位置。
    ( 比如，你可以用服务器的IP地址或者名字来计算哈希值。 )

 3. 映射「数据」到环上
    同样地，我们我们用同样的哈希函数，计算每个数据的key的哈希值，然后把数据也映射到哈希环上的不同位置。

 4. 确定数据归属
    关键，数据存放在哪个节点上呢？ 一致性哈希规定：沿着哈希环顺时针方向查找，遇到的第一个节点，就是存储这个数据的节点。

增加虚拟节点：
防止数据倾斜，每个真实节点映射到哦哈希换上的多个位置。
  - 每个实际节点对应多个虚拟节点，这些虚拟节点均匀地分布在哈希环上
  - key 映射到虚拟节点后，再通过映射关系找到对应的实际节点。

优势总结：
  - 节点增加或减少时，仅影响少部分数据的重新分布。
  - 通过引入虚拟节点，大大提高了数据分配的均衡性。
  - 查找过程使用二分查找（O(log N)复杂度），即使虚拟节点很多，查询也非常高效。

注意：
  - 哈希环（m.keys）只包含虚拟节点的哈希值，用于定位。
  - 真实节点存储在映射表（m.hashMap）里，通过虚拟节点间接参与负载分配。
*/
package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// 定义函数类型 Hash，采取依赖注入的方式，允许用于替换成自定义的 Hash 函数，也方便测试时替换，默认为 crc32.ChecksumIEEE 算法
type Hash func(data []byte) uint32

// Map 包含了一致性哈希算法需要的所有数据结构
type Map struct {
	hash     Hash           // 哈希函数，用于计算节点和 key 的哈希值
	replicas int            // 每个真实节点对应的虚拟节点数量
	keys     []int          // 存放哈希环上所有虚拟节点的哈希值（只记录虚拟节点），保持升序，用来构成哈希环
	hashMap  map[int]string // 从「虚拟节点哈希值」到「真实节点名称」的映射
}

// New 创建并初始化一个Map实例（允许自定义虚拟节点倍数replicas 和 Hash 函数）
// replicas：每个真实节点的虚拟节点个数
// fn：自定义的哈希函数，如果传入nil，则使用默认的 crc32.ChecksumIEEE
func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

// Add 为每个真实节点创建若干虚拟节点，并将这些虚拟节点加入到哈希环中
// - keys 代表想要添加的真实节点名称或标识符
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		// 对于每个真实节点，生成 replicas 个虚拟节点
		for i := 0; i < m.replicas; i++ {
			// 通过 “i + key” 构造不同输入，保证同一真实节点的每个虚拟节点哈希值不同
			// - strconv.Itoa() 用于将整型转换为对应的字符串
			// - []byte() 把字符串转换为 []byte，因为哈希函数接收字节切片
			// - int() 用于将m.hash()得到的uint32类型的哈希值转换为int类型，方便后续存入m.keys切片
			hash := int(m.hash([]byte(strconv.Itoa(i) + key))) // hash 虚拟节点的哈希值 i 当前节点的编号   key 真实节点的名称
			// 将虚拟节点的哈希值追加到 keys 切片
			m.keys = append(m.keys, hash)
			// 在 hashMap 中记录该虚拟节点对应的真实节点
			m.hashMap[hash] = key
		}
	}
	// 对所有虚拟节点的哈希值进行升序排序，构建哈希环
	//（并没有完全创建，因为哈希环的"顺时针查找"行为只在Get中完成）
	sort.Ints(m.keys)
}

// Get 根据传入的key，找到其在哈希环上对应的最近顺时针方向虚拟节点，进而映射到真实节点
// - key    表示你想存储或查找的“数据项”的 key，比如缓存的某个键（如 "user123"）
// - 返回值 返回负责处理这个 key 的真实节点名
func (m *Map) Get(key string) string {
	// 如果环上没有任何节点，直接返回空字符串
	if len(m.keys) == 0 {
		return ""
	}

	// 计算该 key 哈希值
	hash := int(m.hash([]byte(key)))
	// 在已排序的虚拟节点哈希值切片中，二分查找第一个 >= hash 的虚拟节点对应的索引
	//
	// sort.Search函数签名：
	// func Search(n int, f func(int) bool) int
	// - 它会在 [0, n) 区间上做二分查找。
	// 这个方法的用途是找到在范围 [0, n) 中，满足条件 f(i) 为 true 的最小的索引 i。
	// 它假设对于给定的范围，存在一个索引 i，使得当索引小于 i 时 f(i) 为 false，而当索引大于或等于 i 时 f(i) 为 true。
	// 换句话说，函数 f 在 [0, n) 范围内必须是“单调”的，即从某个点开始就一直返回 true。
	// 当所有的f(i) 都是 false 时，则返回 n。
	//
	// 性能是 O(log N)，非常适合在大规模有序数据上定位。
	//
	// 参数的含义：
	// - n = len(m.keys)：我们要在整个虚拟节点哈希值切片中查找
	// - f(i) = (m.keys[i] >= hash)：判断切片第i个虚拟节点的哈希值，是否已经不小于我们要寻找的hash
	//
	//它到底做了什么？
	//  假设 m.keys = [10, 25, 37, 58, 73]（已升序）。
	//  如果 hash = 30，那么：
	//  f(0)= 10>=30? false
	//  f(1)= 25>=30? false
	//  f(2)= 37>=30? true ← 找到第一个 true，立即返回 i=2
	// 因此 idx=2，表示虚拟节点 37 是顺时针第一个落在或经过 30 的节点。
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})

	// 如果hash比m.keys中的所有虚拟节点的哈希值都大，即对所有i，f(i)都是false，sort.Search返回len(m.keys)
	// idx 可能等于 len(m.keys)，此时取 idx % len(m.keys) 会绕回到0，形成环状
	// - 当 idx < len(m.keys) 时，realIdx == idx，不影响正常定位
	// - 当 idx == len(m.keys) 时，realIdx == 0，表示绕回到第一个虚拟节点
	realIdx := idx % len(m.keys)
	// 通过 hashMap 找到该虚拟节点对应的真实节点并返回
	return m.hashMap[m.keys[realIdx]]
}
