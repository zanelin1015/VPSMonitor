package client

import "sync"

var (
	systemVersionOnce sync.Once
	systemVersionText string
)

func currentSystemVersion() string {
	systemVersionOnce.Do(func() {
		systemVersionText = firstNonEmpty(detectSystemVersion(), "Unknown OS")
	})
	return systemVersionText
}
