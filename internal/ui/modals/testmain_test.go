package modals

import (
	"os"
	"testing"

	"github.com/zhubert/plural-core/logger"
)

func TestMain(m *testing.M) {
	// Disable logging during tests to avoid polluting /tmp/plural-debug.log
	logger.Reset()
	logger.Init(os.DevNull)

	// Initialize modal constants for tests
	ModalWidth = 80
	ModalWidthWide = 120
	ModalInputWidth = 72
	ModalInputCharLimit = 256
	IssuesModalMaxVisible = 10

	code := m.Run()

	logger.Reset()
	os.Exit(code)
}
