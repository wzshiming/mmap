// Tests adapted from github.com/edsrzf/mmap-go (BSD-3-Clause), originally from gommap (c) 2010 Gustavo Niemeyer.

package mmap

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// testData returns n non-zero bytes; the prime period 251 keeps every page distinct.
func testData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(1 + i%251)
	}
	return b
}

// testFile writes data to a fresh temporary file and returns it opened with flag, closed at cleanup.
func testFile(t *testing.T, data []byte, flag int) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// testRecover, deferred, reports a panic as a test error instead of aborting the test binary.
func testRecover(t *testing.T) {
	if r := recover(); r != nil {
		t.Errorf("panic: %v", r)
	}
}

func TestReadWrite(t *testing.T) {
	want := testData(64)
	f := testFile(t, want, os.O_RDWR)
	m, err := Map(f, RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if !bytes.Equal(m.Data(), want) {
		t.Error("Data() does not equal the file contents")
	}
	m.Data()[9]++
	want[9]++
	for i := range want {
		if b := m.At(i); b != want[i] {
			t.Errorf("At(%d) = %#x, want %#x", i, b, want[i])
			break
		}
	}
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("file does not show exactly the byte written through Data()")
	}
}

func TestProtFlagsAndErr(t *testing.T) {
	f := testFile(t, testData(64), os.O_RDONLY)
	if m, err := Map(f, RDWR); err == nil {
		m.Close()
		t.Error("Map(RDWR) of a file opened O_RDONLY succeeded, want error")
	}
}

func TestCopy(t *testing.T) {
	data := testData(64)
	f := testFile(t, data, os.O_RDWR)
	m, err := Map(f, COPY)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Data()[10]++
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("file changed through a COPY mapping")
	}
}

func TestMapRegionToEOF(t *testing.T) {
	ps := os.Getpagesize()
	data := testData(2 * ps)
	m, err := MapRegion(testFile(t, data, os.O_RDONLY), RDONLY, ps, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Len() != ps {
		t.Fatalf("MapRegion(offset %d, length -1) of a %d-byte file: Len() = %d, want %d", ps, 2*ps, m.Len(), ps)
	}
	if !bytes.Equal(m.Data(), data[ps:]) {
		t.Error("Data() does not equal the file contents from the offset to EOF")
	}
}

func TestMapRegionLength(t *testing.T) {
	ps := os.Getpagesize()
	data := testData(2 * ps)
	m, err := MapRegion(testFile(t, data, os.O_RDONLY), RDONLY, 0, ps)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Len() != ps {
		t.Fatalf("MapRegion(offset 0, length %d) of a %d-byte file: Len() = %d, want %d", ps, 2*ps, m.Len(), ps)
	}
	if !bytes.Equal(m.Data(), data[:ps]) {
		t.Error("Data() does not equal the first length bytes of the file")
	}
}

func TestMapRegionInvalid(t *testing.T) {
	ps := os.Getpagesize()
	f := testFile(t, testData(2*ps), os.O_RDONLY)
	empty := testFile(t, nil, os.O_RDONLY)
	closed := testFile(t, testData(ps), os.O_RDWR)
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		call    func() (*MMap, error)
		wantErr error
	}{
		{"Map nil file", func() (*MMap, error) { return Map(nil, RDONLY) }, os.ErrInvalid},
		{"MapRegion nil file", func() (*MMap, error) { return MapRegion(nil, RDONLY, 0, 10) }, os.ErrInvalid},
		{"MapRegion closed file", func() (*MMap, error) { return MapRegion(closed, RDWR, 0, ps) }, os.ErrClosed},
		{"MapRegion closed file to EOF", func() (*MMap, error) { return MapRegion(closed, RDONLY, 0, -1) }, os.ErrClosed},
		{"Map closed file", func() (*MMap, error) { return Map(closed, RDONLY) }, os.ErrClosed},
		{"closed file unaligned offset", func() (*MMap, error) { return MapRegion(closed, RDONLY, 1, ps) }, os.ErrClosed},
		{"negative offset", func() (*MMap, error) { return MapRegion(f, RDONLY, -ps, ps) }, nil},
		{"unaligned offset", func() (*MMap, error) { return MapRegion(f, RDONLY, 1, ps) }, nil},
		{"offset at EOF", func() (*MMap, error) { return MapRegion(f, RDONLY, 2*ps, -1) }, nil},
		{"offset beyond EOF", func() (*MMap, error) { return MapRegion(f, RDONLY, 3*ps, -1) }, nil},
		{"zero length", func() (*MMap, error) { return MapRegion(f, RDONLY, 0, 0) }, nil},
		{"Map empty file", func() (*MMap, error) { return Map(empty, RDONLY) }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer testRecover(t)
			m, err := tc.call()
			if err == nil {
				m.Close()
				t.Fatal("got a mapping, want a non-nil error")
			}
			var errno syscall.Errno
			if errors.As(err, &errno) {
				t.Errorf("got syscall error %v, want the arguments rejected before any syscall", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("got error %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestReadAt(t *testing.T) {
	data := testData(64)
	m, err := Map(testFile(t, data, os.O_RDONLY), RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, tc := range []struct {
		name    string
		off     int64
		size    int
		want    []byte
		wantErr error
	}{
		{"full", 0, 64, data, nil},
		{"short", 60, 16, data[60:], io.EOF},
		{"empty at end", 64, 0, nil, nil},
		{"at end", 64, 16, nil, io.EOF},
		{"past end", 65, 16, nil, io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer testRecover(t)
			buf := make([]byte, tc.size)
			n, err := m.ReadAt(buf, tc.off)
			if n != len(tc.want) || err != tc.wantErr {
				t.Fatalf("ReadAt(%d-byte buffer, %d) = (%d, %v), want (%d, %v)", tc.size, tc.off, n, err, len(tc.want), tc.wantErr)
			}
			if !bytes.Equal(buf[:n], tc.want) {
				t.Errorf("ReadAt(%d-byte buffer, %d) read %v, want %v", tc.size, tc.off, buf[:n], tc.want)
			}
		})
	}
	t.Run("negative offset", func(t *testing.T) {
		defer testRecover(t)
		if n, err := m.ReadAt(make([]byte, 16), -1); n != 0 || err == nil || errors.Is(err, io.EOF) {
			t.Errorf("ReadAt(16-byte buffer, -1) = (%d, %v), want (0, an error other than io.EOF)", n, err)
		}
	})
}

func TestWriteAt(t *testing.T) {
	data := testData(64)
	for _, tc := range []struct {
		name    string
		off     int64
		size    int
		wantN   int
		wantErr error
	}{
		{"full", 0, 64, 64, nil},
		{"short", 60, 16, 4, io.ErrShortWrite},
		{"empty at end", 64, 0, 0, nil},
		{"at end", 64, 16, 0, io.ErrShortWrite},
		{"past end", 65, 16, 0, io.ErrShortWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer testRecover(t)
			m, err := Map(testFile(t, data, os.O_RDWR), RDWR)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			// 0xA5 differs from every byte of data, so a write that copies nothing shows.
			src := bytes.Repeat([]byte{0xA5}, tc.size)
			n, err := m.WriteAt(src, tc.off)
			if n != tc.wantN || err != tc.wantErr {
				t.Errorf("WriteAt(%d bytes, %d) = (%d, %v), want (%d, %v)", tc.size, tc.off, n, err, tc.wantN, tc.wantErr)
			}
			want := bytes.Clone(data)
			if tc.wantN > 0 {
				copy(want[tc.off:], src[:tc.wantN])
			}
			if !bytes.Equal(m.Data(), want) {
				t.Errorf("Data() after WriteAt(%d bytes, %d) = %v, want %v", tc.size, tc.off, m.Data(), want)
			}
		})
	}
	t.Run("negative offset", func(t *testing.T) {
		defer testRecover(t)
		m, err := Map(testFile(t, data, os.O_RDWR), RDWR)
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		if n, err := m.WriteAt(make([]byte, 16), -1); n != 0 || err == nil {
			t.Errorf("WriteAt(16 bytes, -1) = (%d, %v), want (0, an error)", n, err)
		}
	})
}

func TestUseAfterClose(t *testing.T) {
	m, err := Map(testFile(t, testData(64), os.O_RDWR), RDWR)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("first Close() = %v, want nil", err)
	}
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"Close", m.Close},
		{"Flush", m.Flush},
		{"Lock", m.Lock},
		{"Unlock", m.Unlock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer testRecover(t)
			if err := tc.call(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("%s() after Close = %v, want os.ErrClosed", tc.name, err)
			}
		})
	}
	t.Run("ReadAt", func(t *testing.T) {
		defer testRecover(t)
		if n, err := m.ReadAt(make([]byte, 1), 0); n != 0 || !errors.Is(err, os.ErrClosed) {
			t.Errorf("ReadAt after Close = (%d, %v), want (0, os.ErrClosed)", n, err)
		}
	})
	t.Run("WriteAt", func(t *testing.T) {
		defer testRecover(t)
		if n, err := m.WriteAt([]byte{1}, 0); n != 0 || !errors.Is(err, os.ErrClosed) {
			t.Errorf("WriteAt after Close = (%d, %v), want (0, os.ErrClosed)", n, err)
		}
	})
	t.Run("Data", func(t *testing.T) {
		defer testRecover(t)
		if d := m.Data(); d != nil {
			t.Errorf("Data() after Close has length %d, want nil", len(d))
		}
	})
	t.Run("Len", func(t *testing.T) {
		defer testRecover(t)
		if n := m.Len(); n != 0 {
			t.Errorf("Len() after Close = %d, want 0", n)
		}
	})
}

func TestFlushLockUnlock(t *testing.T) {
	f := testFile(t, testData(64), os.O_RDWR)
	m, err := Map(f, RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Data()[0] = 0xA5
	if err := m.Flush(); err != nil {
		t.Errorf("Flush() = %v, want nil", err)
	}
	if err := m.Lock(); err != nil {
		t.Errorf("Lock() = %v, want nil", err)
	}
	if err := m.Unlock(); err != nil {
		t.Errorf("Unlock() = %v, want nil", err)
	}
	b := make([]byte, 1)
	if _, err := f.ReadAt(b, 0); err != nil {
		t.Fatal(err)
	}
	if b[0] != 0xA5 {
		t.Errorf("file byte 0 = %#x after Flush, want 0xa5", b[0])
	}
}

func TestCopyReadOnlyFile(t *testing.T) {
	data := testData(64)
	f := testFile(t, data, os.O_RDONLY)
	m, err := Map(f, COPY)
	if err != nil {
		t.Fatal(err)
	}
	m.Data()[0]++
	if err := m.Flush(); err != nil {
		t.Errorf("Flush() = %v, want nil", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("file changed through a COPY mapping")
	}
}

func TestMappingOutlivesFile(t *testing.T) {
	f := testFile(t, testData(64), os.O_RDWR)
	m, err := Map(f, RDWR)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	m.Data()[0] = 0xA5
	if err := m.Flush(); err != nil {
		t.Errorf("Flush() = %v, want nil", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0xA5 {
		t.Errorf("file byte 0 = %#x after Close, want 0xa5", got[0])
	}
}

func TestOffset(t *testing.T) {
	ps := os.Getpagesize()
	data := testData(2 * ps)
	f := testFile(t, data, os.O_RDONLY)
	m, err := MapRegion(f, RDONLY, ps, ps)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Len() != ps {
		t.Fatalf("Len() = %d, want %d", m.Len(), ps)
	}
	if !bytes.Equal(m.Data(), data[ps:]) {
		t.Error("Data() does not equal the file contents from the offset")
	}
	if m, err := MapRegion(f, RDONLY, 1, ps); err == nil {
		m.Close()
		t.Error("MapRegion with offset 1 succeeded, want error")
	}
}
