package nostrfs

import "strconv"

func hexToUint64(hexStr string) uint64 {
	v, _ := strconv.ParseUint(hexStr[0:16], 16, 64)
	return v
}
