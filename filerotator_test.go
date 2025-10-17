package filerotator_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Dizzrt/filerotator"
)

func TestFileRotator(t *testing.T) {
	rotator, err := filerotator.New("./log/logs/{{YYYY}}{{MM}}{{DD}}{{hh}}",
		filerotator.WithSymlink("./log/a/b/c/service.cc"),
		filerotator.WithRotationTime(time.Hour),
		filerotator.WithRotationSize(1024*1024),
	)

	if err != nil {
		t.Fatalf("failed to create file rotator: %v", err)
	}

	for {
		rotator.Write([]byte(fmt.Sprintf("log time %v\n", time.Now())))
		time.Sleep(10 * time.Millisecond)
	}
}
