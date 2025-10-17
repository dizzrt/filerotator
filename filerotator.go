package filerotator

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var patternConversionRegexps = []*regexp.Regexp{
	regexp.MustCompile(`\{\{[^}]+\}\}`),
	regexp.MustCompile(`\*+`),
}

type RotateType uint8

const (
	RotateTypeTime RotateType = iota
	RotateTypeSize
	RotateTypeBoth
)

type unlinkFileInfo struct {
	Path     string
	ModTime  time.Time
	ToUnlink bool
}

type FileRotator struct {
	mu    sync.RWMutex
	outFh *os.File
	clock Clock

	rotateType   RotateType
	rotationTime time.Duration
	rotationSize int64

	maxAge    time.Duration
	maxBackup uint

	fnPattern     string
	fnGlobPattern string
	fnGeneration  uint

	suffix  string
	baseFn  string
	fn      string
	symlink string
}

func New(logFilePath string, opts ...Option) (*FileRotator, error) {
	gp := logFilePath
	for _, re := range patternConversionRegexps {
		gp = re.ReplaceAllString(gp, "*")
	}

	dir, name, suffix := parsePath(logFilePath)
	pattern := filepath.Join(dir, name)

	rotator := &FileRotator{
		mu:    sync.RWMutex{},
		outFh: nil,
		clock: Local,

		rotateType:   RotateTypeBoth,
		rotationTime: time.Hour,
		rotationSize: 10 * 1024 * 1024, // 10 MB

		maxAge:    7 * 24 * time.Hour, // 7 days
		maxBackup: 30,

		fnPattern:     pattern,
		fnGlobPattern: gp,
		fnGeneration:  0,

		suffix:  suffix,
		baseFn:  "",
		fn:      "",
		symlink: "",
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
	newBaseFn := rotator.fnPattern

	if rt == RotateTypeTime || rt == RotateTypeBoth {
		newBaseFn = generateTimeFn(rotator.fnPattern, rotator.rotationTime, rotator.clock)
		if rotator.baseFn != newBaseFn {
			rotator.fnGeneration = 0
		}
	}

	var newFn string
	for {
		newFn = newBaseFn
		if rotator.fnGeneration > 0 {
			newFn += "." + strconv.FormatUint(uint64(rotator.fnGeneration), 10)
		}

		if rotator.suffix != "" {
			newFn += rotator.suffix
		}

		if fi, err := os.Stat(newFn); err != nil {
			if os.IsNotExist(err) {
				break // file does not exist, we can create it
			}

			return nil, errors.Wrapf(err, "failed to check existence of file %v", newFn)
		} else {
			// file exists, check if we need to rotate by size
			if (rt == RotateTypeSize || rt == RotateTypeBoth) && fi.Size() >= rotator.rotationSize && newBaseFn == rotator.baseFn {
				rotator.fnGeneration++
				continue
			}

			break
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

	defer func() {
		fh.Close()
		os.Remove(lockfn)
	}()

	if rotator.symlink != "" {
		tempLinkName := filename + `.symlink`

		linkDest := filename
		linkDir := filepath.Dir(rotator.symlink)

		temp, err := filepath.Rel(linkDir, filename)
		if err == nil {
			linkDest = temp
		}

		if err := os.Symlink(linkDest, tempLinkName); err != nil {
			return errors.Wrap(err, "failed to create new symlink")
		}

		if _, err := os.Stat(linkDir); err != nil {
			if err := os.MkdirAll(linkDir, 0755); err != nil {
				return errors.Wrapf(err, "failed to create directory %s", linkDir)
			}
		}

		if err := os.Rename(tempLinkName, rotator.symlink); err != nil {
			return errors.Wrap(err, "failed to rename new symlink")
		}
	}

	matches, err := filepath.Glob(rotator.fnGlobPattern)
	if err != nil {
		return err
	}

	toUnlinks := make([]string, 0, len(matches))
	toUnlinkMap := make(map[string]unlinkFileInfo, len(matches))

	realMatches := make([]string, 0, len(matches))
	cutoff := rotator.clock.Now().Add(-1 * rotator.maxAge)
	for _, path := range matches {
		if strings.HasSuffix(path, ".lock") || strings.HasSuffix(path, ".symlink") {
			continue // skip lock and symlink files
		}

		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		fl, err := os.Lstat(path)
		if err != nil {
			continue
		}

		if fl.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}

		temp := unlinkFileInfo{
			Path:     path,
			ModTime:  fi.ModTime(),
			ToUnlink: false,
		}

		if rotator.maxAge > 0 && fi.ModTime().Before(cutoff) {
			temp.ToUnlink = true
			toUnlinks = append(toUnlinks, path)
		}

		realMatches = append(realMatches, path)
		toUnlinkMap[path] = temp
	}

	remainingCount := len(realMatches) - len(toUnlinks)
	if rotator.maxBackup > 0 && remainingCount > int(rotator.maxBackup) {
		sort.Slice(realMatches, func(i, j int) bool {
			// sort by modification time, oldest first
			return toUnlinkMap[realMatches[i]].ModTime.Before(toUnlinkMap[realMatches[j]].ModTime)
		})

		for _, path := range realMatches {
			if remainingCount <= int(rotator.maxBackup) {
				break
			}

			temp := toUnlinkMap[path]
			if temp.ToUnlink {
				continue // already marked for unlinking
			}

			toUnlinks = append(toUnlinks, path)
			temp.ToUnlink = true
			toUnlinkMap[path] = temp
			remainingCount--
		}
	}

	if len(toUnlinks) <= 0 {
		return nil
	}

	go func() {
		for _, path := range toUnlinks {
			os.Remove(path)
		}
	}()

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
