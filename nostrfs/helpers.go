package nostrfs

import (
	"fiatjaf.com/nostr"
)

func kindToExtension(kind nostr.Kind) string {
	switch kind {
	case 30023:
		return "md"
	case 30818:
		return "adoc"
	default:
		return "txt"
	}
}
