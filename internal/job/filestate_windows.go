//go:build windows

package job

import (
	"fmt"
	"os"
	"syscall"
)

// extractFileID는 열린 파일 핸들에서 file_id를 추출한다.
// Windows는 GetFileInformationByHandle로 (VolumeSerialNumber,
// FileIndexHigh, FileIndexLow) 조합을 사용. NTFS 상에서 inode 등가물.
func extractFileID(f *os.File) (string, error) {
	var bhfi syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &bhfi); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%d-%d", bhfi.VolumeSerialNumber, bhfi.FileIndexHigh, bhfi.FileIndexLow), nil
}
