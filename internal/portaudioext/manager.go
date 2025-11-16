package portaudioext

import (
	"sync"

	"github.com/gordonklaus/portaudio"
)

var (
	mu       sync.Mutex
	refCount int
)

func Acquire() error {
	mu.Lock()
	defer mu.Unlock()
	if refCount == 0 {
		if err := portaudio.Initialize(); err != nil {
			return err
		}
	}
	refCount++
	return nil
}

func Release() error {
	mu.Lock()
	defer mu.Unlock()
	if refCount == 0 {
		return nil
	}
	refCount--
	if refCount == 0 {
		return portaudio.Terminate()
	}
	return nil
}
