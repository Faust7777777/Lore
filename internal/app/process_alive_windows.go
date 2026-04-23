//go:build windows

package app

import "golang.org/x/sys/windows"

const windowsStillActive = 259

func processAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		switch err {
		case windows.ERROR_INVALID_PARAMETER:
			return false, nil
		case windows.ERROR_ACCESS_DENIED:
			return true, nil
		default:
			return false, err
		}
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false, err
	}
	return code == windowsStillActive, nil
}
