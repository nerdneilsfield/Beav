package cli

// safeBytes converts a possibly negative byte count to a non-negative uint64.
// safeBytes 将可能为负的字节数转换为非负 uint64。
func safeBytes(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
