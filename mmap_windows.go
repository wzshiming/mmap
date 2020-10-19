package mmap

import (
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

func mmap(hfile uintptr, prot int, off, len int) (*MMap, error) {
	flProtect := uint32(syscall.PAGE_READONLY)
	dwDesiredAccess := uint32(syscall.FILE_MAP_READ)
	writable := false
	switch {
	case prot&COPY != 0:
		flProtect = syscall.PAGE_WRITECOPY
		dwDesiredAccess = syscall.FILE_MAP_COPY
		writable = true
	case prot&RDWR != 0:
		flProtect = syscall.PAGE_READWRITE
		dwDesiredAccess = syscall.FILE_MAP_WRITE
		writable = true
	}
	if prot&EXEC != 0 {
		flProtect <<= 4
		dwDesiredAccess |= syscall.FILE_MAP_EXECUTE
	}

	maxSizeHigh := uint32((off + len) >> 32)
	maxSizeLow := uint32(off + len)
	h, errno := syscall.CreateFileMapping(syscall.Handle(hfile), nil, flProtect, maxSizeHigh, maxSizeLow, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	fileOffsetHigh := uint32(off >> 32)
	fileOffsetLow := uint32(off)
	addr, errno := syscall.MapViewOfFile(h, dwDesiredAccess, fileOffsetHigh, fileOffsetLow, uintptr(len))
	if addr == 0 {
		syscall.CloseHandle(h)
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}

	m := &MMap{
		data: ptr2Slice(addr, len),
		extra: extra{
			file:     syscall.Handle(hfile),
			mapview:  h,
			writable: writable,
		},
	}
	return m, nil
}

func (m *MMap) flush() error {
	errno := syscall.FlushViewOfFile(uintptr(unsafe.Pointer(&m.data[0])), uintptr(len(m.data)))
	if errno != nil {
		return os.NewSyscallError("FlushViewOfFile", errno)
	}

	if m.extra.writable {
		if err := syscall.FlushFileBuffers(m.extra.file); err != nil {
			return os.NewSyscallError("FlushFileBuffers", err)
		}
	}

	return nil
}

func (m *MMap) lock() error {
	errno := syscall.VirtualLock(uintptr(unsafe.Pointer(&m.data[0])), uintptr(len(m.data)))
	return os.NewSyscallError("VirtualLock", errno)
}

func (m *MMap) unlock() error {
	errno := syscall.VirtualUnlock(uintptr(unsafe.Pointer(&m.data[0])), uintptr(len(m.data)))
	return os.NewSyscallError("VirtualUnlock", errno)
}

func (m *MMap) unmap() error {
	err := m.flush()
	if err != nil {
		return err
	}

	err = syscall.UnmapViewOfFile(uintptr(unsafe.Pointer(&m.data[0])))
	if err != nil {
		syscall.CloseHandle(m.extra.mapview)
		return os.NewSyscallError("UnmapViewOfFile", err)
	}

	err = syscall.CloseHandle(m.extra.mapview)
	return os.NewSyscallError("CloseHandle", err)
}

func ptr2Slice(ptr uintptr, size int) []byte {
	h := reflect.SliceHeader{
		Data: ptr,
		Len:  size,
		Cap:  size,
	}
	return *(*[]byte)(unsafe.Pointer(&h))
}

type extra struct {
	writable bool
	file     syscall.Handle
	mapview  syscall.Handle
}
