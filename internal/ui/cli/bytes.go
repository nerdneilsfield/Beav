package cli

func safeBytes(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
