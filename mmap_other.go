//go:build !darwin && !linux && !windows

package mmap

import "errors"

func mmap(uintptr, int, int, int) (*MMap, error) {
	return nil, errors.ErrUnsupported
}

func (m *MMap) flush() error { return errors.ErrUnsupported }

func (m *MMap) lock() error { return errors.ErrUnsupported }

func (m *MMap) unlock() error { return errors.ErrUnsupported }

func (m *MMap) unmap() error { return errors.ErrUnsupported }

type extra struct{}
