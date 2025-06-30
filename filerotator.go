package filerotator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type RotateType uint8

const (
	RotateTypeTime RotateType = iota
	RotateTypeSize
	RotateTypeBoth
)

type FileRotator struct {
	mu    sync.RWMutex
	outFh *os.File
	clock Clock

	rotateType   RotateType
	rotationTime time.Duration
	rotationSize int64

	maxAge    time.Duration
	maxBackup uint

	generation uint
	linkName   string
	suffix     string
	patternFn  string
	baseFn     string
	fn         string
}

func New(filename string, opts ...Option) (*FileRotator, error) {
	rotator := &FileRotator{
		mu:    sync.RWMutex{},
		outFh: nil,
		clock: Local,

		rotateType:   RotateTypeBoth,
		rotationTime: time.Hour,
		rotationSize: 10 * 1024 * 1024, // 10 MB

		maxAge:    7 * 24 * time.Hour, // 7 days
		maxBackup: 30,

		generation: 0,
		linkName:   "",
		suffix:     "",
		patternFn:  filename,
		baseFn:     "",
		fn:         "",
	}

	for _, opt := range opts {
		opt(rotator)
	}

	return rotator, nil
}

func (rotator *FileRotator) Write(p []byte) (n int, err error) {
	rotator.mu.Lock()
	defer rotator.mu.Unlock()

	writer, err := rotator.getWriter()
	if err != nil {
		return 0, errors.Wrap(err, "failed to acquite target io.Writer")
	}

	return writer.Write(p)
}

func (rotator *FileRotator) getWriter() (io.Writer, error) {
	rt := rotator.rotateType
	newBaseFn := rotator.patternFn

	if rt == RotateTypeTime || rt == RotateTypeBoth {
		newBaseFn = generateTimeFn(rotator.patternFn, rotator.rotationTime, rotator.clock)
		if rotator.baseFn != newBaseFn {
			rotator.generation = 0
		}
	}

	var newFn string
	for {
		if rotator.suffix == "" {
			if rotator.generation == 0 {
				newFn = newBaseFn
			} else {
				newFn = fmt.Sprintf("%s.%d", newBaseFn, rotator.generation)
			}
		} else {
			if rotator.generation == 0 {
				newFn = fmt.Sprintf("%s.%s", newBaseFn, rotator.suffix)
			} else {
				newFn = fmt.Sprintf("%s.%d.%s", newBaseFn, rotator.generation, rotator.suffix)
			}
		}

		if fi, err := os.Stat(newFn); err != nil {
			if os.IsNotExist(err) {
				break // file does not exist, we can create it
			} else {
				return nil, errors.Wrapf(err, "failed to check existence of file %v", newFn)
			}
		} else {
			// file exists, check if we need to rotate by size
			if (rt == RotateTypeSize || rt == RotateTypeBoth) && fi.Size() >= rotator.rotationSize && newBaseFn == rotator.baseFn {
				rotator.generation++
			} else {
				break
			}
		}
	}

	// return the current fh
	if rotator.fn == newFn {
		return rotator.outFh, nil
	}

	// replace the current fh with a new one
	fh, err := CreateFile(newFn)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create a new file %v", newFn)
	}

	if err := rotator.Rotate(newFn); err != nil {
		err = errors.Wrap(err, "failed to rotate")
		return nil, err
	}

	rotator.outFh.Close()
	rotator.outFh = fh
	rotator.baseFn = newBaseFn
	rotator.fn = newFn

	return fh, nil
}

func (rotator *FileRotator) Rotate(filename string) error {
	lockfn := filename + `.lock`
	fh, err := os.OpenFile(lockfn, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}

	var guard cleanupGuard
	guard.fn = func() {
		fh.Close()
		os.Remove(lockfn)
	}
	defer guard.Run()

	if rotator.linkName != "" {
		tempLinkName := filename + `.symlink`

		linkDest := filename
		linkDir := filepath.Dir(rotator.linkName)

		baseDir := filepath.Dir(filename)
		if strings.Contains(rotator.linkName, baseDir) {
			temp, err := filepath.Rel(linkDir, filename)
			if err != nil {
				return errors.Wrapf(err, "failed to evaluate relative path from %#v to %#v", baseDir, rotator.linkName)
			}

			linkDest = temp
		}

		if err := os.Symlink(linkDest, tempLinkName); err != nil {
			return errors.Wrap(err, "failed to create new symlink")
		}

		_, err := os.Stat(linkDir)
		if err != nil {
			if err := os.MkdirAll(linkDir, 0755); err != nil {
				return errors.Wrapf(err, "failed to create directory %s", linkDir)
			}
		}

		if err := os.Rename(tempLinkName, rotator.linkName); err != nil {
			return errors.Wrap(err, "failed to rename new symlink")
		}
	}

	return nil
}

func (rotator *FileRotator) Close() error {
	rotator.mu.Lock()
	defer rotator.mu.Unlock()

	if rotator.outFh == nil {
		return nil
	}

	err := rotator.outFh.Close()
	rotator.outFh = nil

	return err
}
