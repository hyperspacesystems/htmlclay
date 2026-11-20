//go:build windows

package platform

import "os"

const zoneStream = ":Zone.Identifier"

// keepDownloadMark copies the Zone.Identifier stream. A file with no stream,
// or on a drive with no streams at all (FAT32), has no mark to keep.
func keepDownloadMark(from, to string) error {
	mark, err := os.ReadFile(from + zoneStream)
	if err != nil {
		return nil
	}
	return os.WriteFile(to+zoneStream, mark, 0o644)
}
