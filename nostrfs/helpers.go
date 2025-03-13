package nostrfs

import "strconv"

func kindToExtension(kind int) string {
	switch kind {
	case 30023:
		return "md"
	case 30818:
		return "adoc"
	default:
		return "txt"
	}
}

func hexToUint64(hexStr string) uint64 {
	v, _ := strconv.ParseUint(hexStr[16:32], 16, 64)
	return v
}
