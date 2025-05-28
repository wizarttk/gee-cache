// ByteView.go 用来封装和管理缓存中存储的数据
package geecache

// ByteView结构体 封装了缓存中的数据，并提供了'只读访问接口'「 Len(), ByteSlice(), String() 」
// 设计目的：防止外部直接修改缓存中的数据，确保数据的不可变性。
type ByteView struct { // 起名ByteView的原因：Byte强调存储的额是二进制数据。View强调这是一个不可变的视图，只能读，不能写
	b []byte // 内部数据存储在未导出的字节切片b中。b是私有字段（小写开头），意味着外部代码无法直接访问和修改它，只能通过ByteView提供的方法读取数据
}

// Len 返回 ByteView 中存储的数据的长度
// 该方法主要用于获取数据大小，方便进行内存管理和容量计算
func (v ByteView) Len() int {
	return len(v.b)
}

// ByteSlice 返回存储在 ByteView 中的数据拷贝
// 返回拷贝而不是直接返回内部切片，是为了防止调用者修改缓存内部的数据
func (v ByteView) ByteSlice() []byte {
	// 不直接返回 v.b, 而是通过cloneBytes()放回一个新的字节切片副本的原因：
	// 在 Go 语言中，切片（slice）是一种引用类型，包含指向底层数组的指针、长度和容量等信息。
	// 当你直接返回 v.b 时，虽然 ByteView 是值接收者，复制了结构体，但这种复制是浅拷贝，即只复制了切片的描述符，而底层数组仍然共享。因此，外部代码通过返回的切片可以修改底层数组，影响缓存数据。
	// 为了避免这种情况，ByteSlice 方法使用 cloneBytes(v.b) 返回一个新的字节切片副本。这样，外部代码对返回的切片进行修改时，不会影响 ByteView 内部的原始数据，从而保证数据的只读性和安全性。
	return cloneBytes(v.b)
}

// String 将 ByteView 中存储数据转换成字符串并返回
// 这样可以方便将缓存中的二进制数据以字符串的形式展示或打印
func (v ByteView) String() string {
	return string(v.b)
}

// cloneBytes 创建并返回传入字节切片的一个深拷贝，确保在返回数据时，不会暴露内部的原始数据引用
// 结果：返回一个和原数据内容一样但不共享底层数组的副本。
func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b)) // 创建一个长度相同的新切片
	copy(c, b)
	return c
}
