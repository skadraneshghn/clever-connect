//go:build !linux

package torrent

import "os"

// punchHole deallocates the data blocks of the file at path while keeping its
// logical size, turning it into a sparse stub. On non-Linux platforms (e.g.
// Windows dev) there is no portable atomic punch-hole, so this falls back to
// the truncate rebuild: truncate to 0 (frees all blocks) then back to size
// (pure sparse). The window between the two truncates is tiny and only
// matters for anacrolix's size check, which is not the production path here.
func punchHole(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := fi.Size()
	if size == 0 {
		return nil
	}
	return truncateToSparse(path, size)
}
