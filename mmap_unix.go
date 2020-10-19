// +build darwin dragonfly freebsd linux openbsd solaris netbsd

package mmap

import (
	"syscall"
	"unsafe"
)

func mmap(fd uintptr, inprot int, off, len int) (*MMap, error) {
	flags := syscall.MAP_SHARED
	prot := syscall.PROT_READ
	switch {
	case inprot&COPY != 0:
		prot |= syscall.PROT_WRITE
		flags = syscall.MAP_PRIVATE
	case inprot&RDWR != 0:
		prot |= syscall.PROT_WRITE
	}
	if inprot&EXEC != 0 {
		prot |= syscall.PROT_EXEC
	}

	b, err := syscall.Mmap(int(fd), int64(off), len, prot, flags)
	if err != nil {
		return nil, err
	}

	m := &MMap{
		data: b,
	}
	return m, nil
}

func (m *MMap) flush() error {
	_, _, err := syscall.Syscall(syscall.SYS_MSYNC, uintptr(unsafe.Pointer(&m.data[0])), uintptr(len(m.data)), syscall.MS_SYNC)
	if err != 0 {
		return err
	}
	return nil
}

func (m *MMap) lock() error {
	return syscall.Mlock(m.data)
}

func (m *MMap) unlock() error {
	return syscall.Munlock(m.data)
}

func (m *MMap) unmap() error {
	return syscall.Munmap(m.data)
}

type extra struct{}
