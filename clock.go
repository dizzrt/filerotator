package filerotator

import "time"

type Clock interface {
	Now() time.Time
}

type clockFn func() time.Time

func (c clockFn) Now() time.Time {
	return c()
}

var UTC = clockFn(func() time.Time { return time.Now().UTC() })
var Local = clockFn(func() time.Time { return time.Now() })
