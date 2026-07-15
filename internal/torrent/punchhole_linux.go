//go:build linux

package torrent

import (
	"os"

	"golang.org/x/sys/unix"
)

// punchHole deallocates the data blocks of the file at path while keeping its
// logical size, turning it into a sparse stub. It is used after a torrent file
// is archived to S3 so anacrolix's checkCompleteFileSizes size check still
// passes (no re-download) while the ephemeral disk is freed.
//
// On Linux this is an atomic fallocate(FALLOC_FL_PUNCH_HOLE | KEEP_SIZE): the
// file may be open by anacrolix on a separate file descriptor because
// punch-hole operates on the inode, not the handle. If the filesystem does not
// support punching holes it falls back to the portable truncate rebuild.
func punchHole(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	if size == 0 {
		return nil
	}

	mode := unix.FALLOC_FL_PUNCH_HOLE | unix.FALLOC_FL_KEEP_SIZE
	if err := unix.Fallocate(int(f.Fd()), uint32(mode), 0, size); err != nil {
		return truncateToSparse(path, size)
	}
	return nil
}
