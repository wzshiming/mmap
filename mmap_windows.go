package mmap

import (
	"os"
	"syscall"
	"unsafe"
)

// MapViewOfFile offsets must be multiples of the allocation granularity, which can exceed the page size.
var allocationGranularity = func() int {
	var info struct {
		processorArchitecture     uint16
		reserved                  uint16
		pageSize                  uint32
		minimumApplicationAddress uintptr
		maximumApplicationAddress uintptr
		activeProcessorMask       uintptr
		numberOfProcessors        uint32
		processorType             uint32
		allocationGranularity     uint32
		processorLevel            uint16
		processorRevision         uint16
	}
	syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemInfo").Call(uintptr(unsafe.Pointer(&info)))
	return int(info.allocationGranularity)
}()

func mmap(hfile uintptr, prot int, off, len int) (*MMap, error) {
	flProtect := uint32(syscall.PAGE_READONLY)
	dwDesiredAccess := uint32(syscall.FILE_MAP_READ)
	switch {
	case prot&COPY != 0:
		flProtect = syscall.PAGE_WRITECOPY
		dwDesiredAccess = syscall.FILE_MAP_COPY
	case prot&RDWR != 0:
		flProtect = syscall.PAGE_READWRITE
		dwDesiredAccess = syscall.FILE_MAP_WRITE
	}
	if prot&EXEC != 0 {
		flProtect <<= 4
		dwDesiredAccess |= syscall.FILE_MAP_EXECUTE
	}

	maxSize := int64(off) + int64(len)
	h, errno := syscall.CreateFileMapping(syscall.Handle(hfile), nil, flProtect, uint32(maxSize>>32), uint32(maxSize), nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}

	delta := off % allocationGranularity
	viewOffset := int64(off - delta)
	base, errno := syscall.MapViewOfFile(h, dwDesiredAccess, uint32(viewOffset>>32), uint32(viewOffset), uintptr(delta+len))
	if base == 0 {
		syscall.CloseHandle(h)
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}

	var file syscall.Handle
	if dwDesiredAccess&syscall.FILE_MAP_WRITE != 0 {
		process, _ := syscall.GetCurrentProcess()
		err := syscall.DuplicateHandle(process, syscall.Handle(hfile), process, &file, 0, false, syscall.DUPLICATE_SAME_ACCESS)
		if err != nil {
			syscall.UnmapViewOfFile(base)
			syscall.CloseHandle(h)
			return nil, os.NewSyscallError("DuplicateHandle", err)
		}
	}

	m := &MMap{
		// Reinterpreted through its address: vet rejects unsafe.Pointer(base) for a uintptr variable.
		data: unsafe.Slice((*byte)(*(*unsafe.Pointer)(unsafe.Pointer(&base))), delta+len)[delta:],
		extra: extra{
			base:    base,
			mapview: h,
			file:    file,
		},
	}
	return m, nil
}

func (m *MMap) flush() error {
	errno := syscall.FlushViewOfFile(uintptr(unsafe.Pointer(&m.data[0])), uintptr(len(m.data)))
	if errno != nil {
		return os.NewSyscallError("FlushViewOfFile", errno)
	}

	if m.extra.file != 0 {
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

// A retried Close must skip released steps: closed handle values can be reused.
func (m *MMap) unmap() error {
	var err error
	if m.extra.base != 0 {
		if e := syscall.UnmapViewOfFile(m.extra.base); e == nil {
			m.extra.base = 0
		} else {
			err = os.NewSyscallError("UnmapViewOfFile", e)
		}
	}
	if m.extra.mapview != 0 {
		if e := syscall.CloseHandle(m.extra.mapview); e == nil {
			m.extra.mapview = 0
		} else if err == nil {
			err = os.NewSyscallError("CloseHandle", e)
		}
	}
	if m.extra.file != 0 {
		if e := syscall.CloseHandle(m.extra.file); e == nil {
			m.extra.file = 0
		} else if err == nil {
			err = os.NewSyscallError("CloseHandle", e)
		}
	}
	return err
}

type extra struct {
	base    uintptr // view start; data begins later when the offset is not granularity-aligned
	mapview syscall.Handle
	file    syscall.Handle // duplicate of the file handle for RDWR mappings, else 0
}
