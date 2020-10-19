package mmap

import (
	"fmt"
	"os"
)

const (
	// RDONLY maps the memory read-only.
	// Attempts to write to the MMap object will result in undefined behavior.
	RDONLY = 0
	// RDWR maps the memory as read-write. Writes to the MMap object will update the
	// underlying file.
	RDWR = 1 << iota
	// COPY maps the memory as copy-on-write. Writes to the MMap object will affect
	// memory, but the underlying file will remain unchanged.
	COPY
	// If EXEC is set, the mapped memory is marked as executable.
	EXEC
)

// MMap represents a file mapped into memory.
type MMap struct {
	data []byte
	extra
}

// Map maps an entire file into memory.
func Map(f *os.File, prot int) (*MMap, error) {
	return MapRegion(f, prot, 0, -1)
}

// MapRegion maps part of a file into memory.
// The offset parameter must be a multiple of the system's page size.
func MapRegion(f *os.File, prot int, offset, length int) (*MMap, error) {
	pageSize := os.Getpagesize()
	if offset%pageSize != 0 {
		return nil, fmt.Errorf("offset parameter must be a multiple of the system's page size %d", pageSize)
	}

	if length < 0 {
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}
		length = int(fi.Size())
	}

	return mmap(f.Fd(), prot, offset, length)
}

// Data returns mapped memory.
func (m *MMap) Data() []byte {
	return m.data
}

// Lock keeps the mapped region in physical memory.
func (m MMap) Lock() error {
	return m.lock()
}

// Unlock reverses the effect of Lock.
func (m MMap) Unlock() error {
	return m.unlock()
}

// Flush synchronizes the mapping's contents to the file's contents on disk.
func (m MMap) Flush() error {
	return m.flush()
}

// Close implements the io.Closer interface.
func (m *MMap) Close() error {
	err := m.unmap()
	m.data = nil
	return err
}

// ReadAt implements the io.ReaderAt interface.
func (m *MMap) ReadAt(dest []byte, offset int64) (int, error) {
	return copy(dest, m.data[offset:]), nil
}

// WriteAt implements the io.WriterAt interface.
func (m *MMap) WriteAt(src []byte, offset int64) (int, error) {
	return copy(m.data[offset:], src), nil
}

// Len returns the length of the underlying mapped memory.
func (m *MMap) Len() int {
	return len(m.data)
}

// At returns the byte at index i.
func (m *MMap) At(i int) byte {
	return m.data[i]
}
