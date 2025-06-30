package filerotator

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var patterns = map[string]string{
	"{{YYYY}}": "2006",
	"{{MM}}":   "01",
	"{{DD}}":   "02",
	"{{hh}}":   "15",
	"{{mm}}":   "04",
	"{{ss}}":   "05",
}

func generateTimeFn(patternFn string, rotationTime time.Duration, clock Clock) string {
	for p, v := range patterns {
		patternFn = strings.ReplaceAll(patternFn, p, v)
	}

	now := clock.Now()
	var base time.Time
	if now.Location() != time.UTC {
		base = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
		base = base.Truncate(rotationTime)
		base = time.Date(base.Year(), base.Month(), base.Day(), base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), base.Location())
	} else {
		base = now.Truncate(rotationTime)
	}

	patternFn = base.Format(patternFn)
	return patternFn
}

func CreateFile(filename string) (*os.File, error) {
	dirname := filepath.Dir(filename)
	if err := os.MkdirAll(dirname, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create directory %s", dirname)
	}

	fh, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Errorf("failed to open file %s: %s", filename, err)
	}

	return fh, nil
}
