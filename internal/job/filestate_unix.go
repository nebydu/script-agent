//go:build unix

package job

import (
	"os"
	"strconv"
	"syscall"
)

// extractFileID는 열린 파일 핸들에서 file_id를 추출한다.
// POSIX는 inode 번호를 사용 (rename된 파일은 inode 보존, 새 파일은 새 inode).
func extractFileID(f *os.File) (string, error) {
	var st syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &st); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(st.Ino), 10), nil
}
