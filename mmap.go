package mmap

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
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

var errNegativeOffset = errors.New("mmap: negative offset")

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
// f must be open (a closed f yields os.ErrClosed).
// offset must be >= 0 and a multiple of the page size; length < 0 maps from offset to EOF,
// otherwise length must be > 0; an explicit length may extend past EOF (on Windows an RDWR
// mapping grows the file, while RDONLY and COPY mappings past EOF fail). On darwin and linux such a
// mapping succeeds, but accessing a page entirely beyond EOF raises SIGBUS and aborts the process.
func MapRegion(f *os.File, prot int, offset, length int) (*MMap, error) {
	if f == nil {
		return nil, os.ErrInvalid
	}
	// Fd is ^uintptr(0) after Close, which Windows would silently map as pagefile memory.
	fd := f.Fd()
	if fd == ^uintptr(0) {
		return nil, os.ErrClosed
	}
	if offset < 0 {
		return nil, errNegativeOffset
	}
	pageSize := os.Getpagesize()
	if offset%pageSize != 0 {
		return nil, fmt.Errorf("mmap: offset parameter must be a multiple of the system's page size %d", pageSize)
	}

	if length < 0 {
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}
		n := fi.Size() - int64(offset)
		if n <= 0 || n > math.MaxInt {
			return nil, fmt.Errorf("mmap: cannot map %d bytes from offset %d to EOF", n, offset)
		}
		length = int(n)
	}
	if length == 0 {
		return nil, errors.New("mmap: length must be greater than 0")
	}

	m, err := mmap(fd, prot, offset, length)
	runtime.KeepAlive(f)
	return m, err
}

// Data returns mapped memory.
// The slice must not be used after Close.
func (m *MMap) Data() []byte {
	return m.data
}

// Lock keeps the mapped region in physical memory.
func (m MMap) Lock() error {
	if m.data == nil {
		return os.ErrClosed
	}
	return m.lock()
}

// Unlock reverses the effect of Lock.
func (m MMap) Unlock() error {
	if m.data == nil {
		return os.ErrClosed
	}
	return m.unlock()
}

// Flush synchronizes the mapping's contents to the file's contents on disk.
func (m MMap) Flush() error {
	if m.data == nil {
		return os.ErrClosed
	}
	return m.flush()
}

// Close implements the io.Closer interface.
// It does not flush; after it succeeds, Close, Flush, Lock, Unlock, ReadAt and WriteAt return os.ErrClosed.
func (m *MMap) Close() error {
	if m.data == nil {
		return os.ErrClosed
	}
	if err := m.unmap(); err != nil {
		return err
	}
	m.data = nil
	return nil
}

// ReadAt implements the io.ReaderAt interface.
// It returns io.EOF when fewer than len(dest) bytes are read.
func (m *MMap) ReadAt(dest []byte, offset int64) (int, error) {
	if m.data == nil {
		return 0, os.ErrClosed
	}
	if offset < 0 {
		return 0, errNegativeOffset
	}
	if offset > int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(dest, m.data[offset:])
	if n < len(dest) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt implements the io.WriterAt interface.
// It returns io.ErrShortWrite when fewer than len(src) bytes are written.
func (m *MMap) WriteAt(src []byte, offset int64) (int, error) {
	if m.data == nil {
		return 0, os.ErrClosed
	}
	if offset < 0 {
		return 0, errNegativeOffset
	}
	if offset > int64(len(m.data)) {
		return 0, io.ErrShortWrite
	}
	n := copy(m.data[offset:], src)
	if n < len(src) {
		return n, io.ErrShortWrite
	}
	return n, nil
}

// Len returns the length of the underlying mapped memory.
func (m *MMap) Len() int {
	return len(m.data)
}

// At returns the byte at index i.
func (m *MMap) At(i int) byte {
	return m.data[i]
}
