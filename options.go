package filerotator

import "time"

type Option func(*FileRotator)

func WithClock(clock Clock) Option {
	return func(rotator *FileRotator) {
		rotator.clock = clock
	}
}

func WithRotateType(rotateType RotateType) Option {
	return func(rotator *FileRotator) {
		rotator.rotateType = rotateType
	}
}

func WithRotationTime(rotationTime time.Duration) Option {
	return func(rotator *FileRotator) {
		rotator.rotationTime = rotationTime
	}
}

func WithRotationSize(rotationSize int64) Option {
	return func(rotator *FileRotator) {
		rotator.rotationSize = rotationSize
	}
}

func WithMaxAge(maxAge time.Duration) Option {
	return func(rotator *FileRotator) {
		rotator.maxAge = maxAge
	}
}

func WithMaxBackup(maxBackup uint) Option {
	return func(rotator *FileRotator) {
		rotator.maxBackup = maxBackup
	}
}

func WithSymlink(symlink string) Option {
	return func(rotator *FileRotator) {
		rotator.symlink = symlink
	}
}
