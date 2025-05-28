package consistenthash

import (
	"strconv"
	"testing"
)

// TestHashing 测试一致性哈希在添加节点前后，对不同 key 的映射是否符合预期
func TestHashing(t *testing.T) {
	// 构造一个 Map 实例：每个真实节点有 3 个虚拟节点，
	// 哈希函数直接把传入的字节（ASCII 数字）转成对应的整数值
	hash := New(3, func(key []byte) uint32 {
		i, _ := strconv.Atoi(string(key)) // 把字节切片转成字符串，再转成整数
		return uint32(i)                  // 返回该整数作为“哈希值”
	})

	// 由于 replicas=3，对于每个真实节点 X，会生成虚拟节点：
	// hash(“0X”)、hash(“1X”)、hash(“2X”)，也就是 0*10+X、1*10+X、2*10+X
	// Add("6","4","2") 后，这些虚拟节点的哈希值（int）依次是：
	// “6” → 6,16,26； “4” → 4,14,24； “2” → 2,12,22
	// 合并排序后 m.keys = [2,4,6,12,14,16,22,24,26]
	hash.Add("6", "4", "2")

	// 构造若干测试用例：map[key]expectedNode
	// 意思是：调用 hash.Get(key) 应该返回 expectedNode
	testCases := map[string]string{
		"2":  "2", // 2 的哈希是 2，正好命中虚拟节点 2→真实节点 "2"
		"11": "2", // 11 的哈希是 11，顺时针第一个 ≥11 的虚拟节点是 12→"2"
		"23": "4", // 23 的哈希是 23，顺时针第一个 ≥23 的虚拟节点是 24→"4"
		"27": "2", // 27 的哈希是 27，超过最大 26，回绕到第一个 2→"2"
	}

	// 遍历所有测试用例，验证 Get 结果
	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}

	// 现在再添加一个真实节点 "8"，它会生成虚拟节点 8,18,28
	hash.Add("8")

	// 由于新增了 28 这个虚拟节点，27 的顺时针第一个 ≥27 的位置现在是 28→"8"
	testCases["27"] = "8"

	// 重新遍历测试用例，包含更新后的 "27" 映射，确保新增节点生效
	for k, v := range testCases {
		if hash.Get(k) != v {
			t.Errorf("Asking for %s, should have yielded %s", k, v)
		}
	}
}
