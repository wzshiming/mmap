package mmap

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var testData = make([]byte, 1024)
var testPath = filepath.Join(os.TempDir(), "mmap_test_data")

func openFile(flags int) *os.File {
	f, err := os.OpenFile(testPath, flags, 0644)
	if err != nil {
		panic(err)
	}
	return f
}

func init() {
	rand.Read(testData)
	f := openFile(os.O_RDWR | os.O_CREATE | os.O_TRUNC)
	f.Write(testData)
	f.Close()
}

func TestReadWrite(t *testing.T) {
	f := openFile(os.O_RDWR)
	defer f.Close()
	mmap, err := Map(f, RDWR)
	if err != nil {
		t.Fatal(err)
	}
	defer mmap.Close()
	if !bytes.Equal(testData, mmap.Data()) {
		t.Errorf("mmap != testData: %q, %q", mmap.Data(), testData)
	}

	index := 9
	data := mmap.Data()
	r := data[index]
	data[index]++
	defer func() {
		data[index] = r
	}()

	fileData, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileData[:index], testData[:index]) ||
		!bytes.Equal(fileData[index+1:], testData[index+1:]) ||
		fileData[index] == testData[index] {
		t.Errorf("file wasn't modified")
	}

}

func TestProtFlagsAndErr(t *testing.T) {
	f := openFile(os.O_RDONLY)
	defer f.Close()
	if _, err := Map(f, RDWR); err == nil {
		t.Errorf("expected error")
	}
}

func TestCopy(t *testing.T) {
	f := openFile(os.O_RDWR)
	defer f.Close()
	mmap, err := Map(f, COPY)
	if err != nil {
		t.Fatal(err)
	}
	defer mmap.Close()

	index := 10
	data := mmap.Data()
	data[index]++

	fileData, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileData, testData) {
		t.Errorf("file was modified")
	}
}

func TestOffset(t *testing.T) {
	const pageSize = 65536

	bigFilePath := filepath.Join(os.TempDir(), "nonzero")
	fileobj, err := os.OpenFile(bigFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = fileobj.Truncate(2 * pageSize)
	if err != nil {
		t.Fatal(err)
	}
	err = fileobj.Close()
	if err != nil {
		t.Fatal(err)
	}

	fileobj, err = os.OpenFile(bigFilePath, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MapRegion(fileobj, RDONLY, 0, pageSize)
	if err != nil {
		t.Fatal(err)
	}
	err = m.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = fileobj.Close()
	if err != nil {
		t.Fatal(err)
	}

	fileobj, err = os.OpenFile(bigFilePath, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	m, err = MapRegion(fileobj, RDONLY, pageSize, pageSize)
	if err != nil {
		t.Fatal(err)
	}
	err = m.Close()
	if err != nil {
		t.Fatal(err)
	}
	m, err = MapRegion(fileobj, RDONLY, 1, pageSize)
	if err == nil {
		t.Error("expect error because offset is not multiple of page size")
	}
}
