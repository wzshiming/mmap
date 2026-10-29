# mmap

[![GoDoc](https://godoc.org/github.com/wzshiming/mmap?status.svg)](https://godoc.org/github.com/wzshiming/mmap)
[![GitHub license](https://img.shields.io/github/license/wzshiming/mmap.svg)](https://github.com/wzshiming/mmap/blob/main/LICENSE)

mmap syscall function exposes the mapped memory as array of bytes

## Platforms

Supported on linux, darwin and windows; on other platforms the package compiles and `Map` returns `errors.ErrUnsupported`.

## License

Licensed under the [MIT License](https://github.com/wzshiming/mmap/blob/main/LICENSE). Portions are derived from [edsrzf/mmap-go](https://github.com/edsrzf/mmap-go) and are distributed under the BSD 3-Clause License; see LICENSE.
