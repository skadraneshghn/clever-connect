package torrent

import "os"

// truncateToSparse frees every data block of the file at path and recreates it
// as a sparse (zero-block) file of the given size. It is portable but not
// atomic: a stat between the two truncates would observe size 0, so it is only
// used as a fallback where fallocate punch-hole is unavailable.
func truncateToSparse(path string, size int64) error {
	if err := os.Truncate(path, 0); err != nil {
		return err
	}
	return os.Truncate(path, size)
}
