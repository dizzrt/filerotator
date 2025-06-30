package filerotator

import "sync"

type cleanupGuard struct {
	enable bool
	fn     func()
	mu     sync.Mutex
}

func (g *cleanupGuard) Enable() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.enable = true
}

func (g *cleanupGuard) Run() {
	g.fn()
}
