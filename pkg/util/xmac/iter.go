package xmac

import "iter"

// Range 返回从 from 到 to（包含）的 MAC 地址迭代器。
// 如果 from > to，返回空迭代器。
// 如果迭代过程中发生溢出（到达 ff:ff:ff:ff:ff:ff），自动终止。
//
// 零值地址可以参与迭代（视为 00:00:00:00:00:00）。
// 对于大范围迭代，建议配合 [CollectN] 的 maxCount 参数或使用 [RangeN] 限制数量。
//
// 示例：
//
//	from := xmac.MustParse("00:00:00:00:00:01")
//	to := xmac.MustParse("00:00:00:00:00:05")
//	for addr := range xmac.Range(from, to) {
//	    fmt.Println(addr)
//	}
func Range(from, to Addr) iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		// 无效范围检查
		if from.Compare(to) > 0 {
			return
		}

		current := from
		for {
			if !yield(current) {
				return
			}
			// 到达终点
			if current == to {
				return
			}
			// 设计决策: 防御性分支——在当前逻辑下 Next() 不会返回 ErrOverflow，
			// 因为 from<=to 且 current==to 时已提前返回。保留此分支以防止
			// 未来修改 Compare 或终止条件时引入溢出风险。
			next, err := current.Next()
			if err != nil {
				return
			}
			current = next
		}
	}
}

// RangeN 返回从 start 开始的 n 个连续 MAC 地址的迭代器。
// 如果 n <= 0，返回空迭代器。
// 如果在到达 n 个地址之前发生溢出，迭代器提前终止。
//
// 示例：
//
//	start := xmac.MustParse("00:00:00:00:00:fe")
//	for addr := range xmac.RangeN(start, 5) {
//	    fmt.Println(addr)
//	}
func RangeN(start Addr, n int) iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		if n <= 0 {
			return
		}

		current := start
		remaining := n
		for remaining > 0 {
			if !yield(current) {
				return
			}
			remaining--
			// 如果还有更多要迭代，获取下一个
			if remaining > 0 {
				next, err := current.Next()
				if err != nil {
					// 溢出，终止迭代
					return
				}
				current = next
			}
		}
	}
}

// RangeWithIndex 返回带索引的 MAC 地址范围迭代器。
// 索引从 0 开始。索引类型为 int，与 [slices.All] 保持一致（参见 doc.go 平台要求）。
//
// 示例：
//
//	from := xmac.MustParse("00:00:00:00:00:01")
//	to := xmac.MustParse("00:00:00:00:00:05")
//	for i, addr := range xmac.RangeWithIndex(from, to) {
//	    fmt.Printf("%d: %s\n", i, addr)
//	}
func RangeWithIndex(from, to Addr) iter.Seq2[int, Addr] {
	return func(yield func(int, Addr) bool) {
		// 无效范围检查
		if from.Compare(to) > 0 {
			return
		}

		current := from
		index := 0
		for {
			if !yield(index, current) {
				return
			}
			// 到达终点
			if current == to {
				return
			}
			// 设计决策: 防御性分支——在当前逻辑下 Next() 不会返回 ErrOverflow，
			// 因为 from<=to 且 current==to 时已提前返回。保留此分支以防止
			// 未来修改 Compare 或终止条件时引入溢出风险。
			next, err := current.Next()
			if err != nil {
				return
			}
			current = next
			index++
		}
	}
}

// CollectN 将迭代器中的地址收集到切片中，最多收集 maxCount 个。
// maxCount ≤ 0 表示不限制数量。
//
// 设计决策: 命名为 CollectN 而非 Collect，避免与 Go 1.23+ 标准库 [slices.Collect] 混淆。
// 不限制数量时建议直接使用 [slices.Collect]。
//
// 性能提示：maxCount > 0 时会预分配切片容量（上限 1<<20 防止极端值 OOM）；
// maxCount ≤ 0 时切片从零开始增长，对于已知大小的范围，
// 建议传入 maxCount 或使用 [RangeCount] 估算后传入。
//
// 示例：
//
//	from := xmac.MustParse("00:00:00:00:00:01")
//	to := xmac.MustParse("00:00:00:00:00:05")
//	addrs := xmac.CollectN(xmac.Range(from, to), 100)
func CollectN(seq iter.Seq[Addr], maxCount int) []Addr {
	var result []Addr
	if maxCount > 0 {
		// 设计决策: 限制预分配上限为 1<<20（约 100 万），防止极端 maxCount 导致 OOM。
		// 超过上限时仍可正确收集，只是底层 slice 需要动态扩容。
		result = make([]Addr, 0, min(maxCount, 1<<20))
	}
	count := 0
	for addr := range seq {
		if maxCount > 0 && count >= maxCount {
			break
		}
		result = append(result, addr)
		count++
	}
	return result
}

// Count 返回迭代器中的地址数量。
// 注意：这会消耗整个迭代器。
// 对于大范围，考虑使用 [RangeCount] 函数直接计算（返回 uint64，无溢出风险）。
func Count(seq iter.Seq[Addr]) int {
	count := 0
	for range seq {
		count++
	}
	return count
}

// RangeCount 计算从 from 到 to（包含两端）的地址数量。
// 如果 from > to，返回 0。
// 返回 uint64 以支持大范围（最大 2^48 个地址）。
//
// 零值地址可以参与计算（视为 00:00:00:00:00:00）。
// 注意：RangeCount(Addr{}, Addr{}) 返回 1——零值地址虽然 [Addr.IsValid] 返回 false，
// 但作为 48 位地址空间中的合法数值（0x000000000000）参与数学运算。
func RangeCount(from, to Addr) uint64 {
	if from.Compare(to) > 0 {
		return 0
	}
	// MAC 地址最多 48 位，可以用 uint64 表示差值
	fromVal := addrToUint64(from)
	toVal := addrToUint64(to)
	return toVal - fromVal + 1
}

// addrToUint64 将 MAC 地址转换为 uint64（用于计算）。
func addrToUint64(a Addr) uint64 {
	b := a.bytes
	return uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
}

// RangeReverse 返回从 from 到 to（包含）的 MAC 地址反向迭代器。
// 迭代顺序为从 to 到 from（递减）。
// 如果 from > to，返回空迭代器。
// 如果迭代过程中发生下溢（到达 00:00:00:00:00:00），自动终止。
//
// 零值地址可以参与迭代（视为 00:00:00:00:00:00）。
//
// 示例：
//
//	from := xmac.MustParse("00:00:00:00:00:01")
//	to := xmac.MustParse("00:00:00:00:00:05")
//	for addr := range xmac.RangeReverse(from, to) {
//	    fmt.Println(addr)  // 输出: 05, 04, 03, 02, 01
//	}
func RangeReverse(from, to Addr) iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		// 无效范围检查
		if from.Compare(to) > 0 {
			return
		}

		current := to
		for {
			if !yield(current) {
				return
			}
			// 到达起点
			if current == from {
				return
			}
			// 设计决策: 防御性分支——在当前逻辑下 Prev() 不会返回 ErrUnderflow，
			// 因为 from<=to 且 current==from 时已提前返回。保留此分支以防止
			// 未来修改 Compare 或终止条件时引入下溢风险。
			prev, err := current.Prev()
			if err != nil {
				return
			}
			current = prev
		}
	}
}

// RangeReverseWithIndex 返回带索引的 MAC 地址反向范围迭代器。
// 索引从 0 开始，表示从 to 开始的偏移量。
//
// 示例：
//
//	from := xmac.MustParse("00:00:00:00:00:01")
//	to := xmac.MustParse("00:00:00:00:00:05")
//	for i, addr := range xmac.RangeReverseWithIndex(from, to) {
//	    fmt.Printf("%d: %s\n", i, addr)  // 0: 05, 1: 04, ...
//	}
func RangeReverseWithIndex(from, to Addr) iter.Seq2[int, Addr] {
	return func(yield func(int, Addr) bool) {
		// 无效范围检查
		if from.Compare(to) > 0 {
			return
		}

		current := to
		index := 0
		for {
			if !yield(index, current) {
				return
			}
			// 到达起点
			if current == from {
				return
			}
			// 设计决策: 防御性分支——在当前逻辑下 Prev() 不会返回 ErrUnderflow，
			// 因为 from<=to 且 current==from 时已提前返回。保留此分支以防止
			// 未来修改 Compare 或终止条件时引入下溢风险。
			prev, err := current.Prev()
			if err != nil {
				return
			}
			current = prev
			index++
		}
	}
}
