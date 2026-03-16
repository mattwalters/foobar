package tui

import (
	"bytes"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
)

func TestApp_BasicRender(t *testing.T) {
	// We pass a dummy socket path; the HTTP client will fail to connect,
	// which is exactly what we expect. We just want to ensure it renders the basic skeleton.
	m := New("/tmp/dummy.sock")

	// Start the model using the teatest harness with a fixed mocked terminal size
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	// Allow some time for the initial render and the tick messages
	time.Sleep(100 * time.Millisecond)

	// Verify that the UI renders the core "Processes" title block
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Processes"))
	}, teatest.WithDuration(time.Second))

	// Test sending keypresses to the UI
	tm.Type("j") // down arrow

	time.Sleep(50 * time.Millisecond)

	// Send a quit message
	tm.Type("q")

	// Wait for the program to exit cleanly
	tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}
