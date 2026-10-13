package gamepad

import (
	"errors"
	"sync"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func init() {
	Register("mock", func() (Gamepad, error) { return NewMock(), nil })
}

// ErrNotConnected is returned when a backend is used before Connect.
var ErrNotConnected = errors.New("gamepad: not connected")

// Mock is an in-memory backend for development and tests. The host never
// sees it.
type Mock struct {
	mu        sync.Mutex
	connected bool
	state     protocol.ControllerState
	updates   int
	rumble    RumbleHandler
}

// NewMock returns a disconnected Mock.
func NewMock() *Mock { return &Mock{} }

func (m *Mock) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *Mock) Update(s protocol.ControllerState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.connected {
		return ErrNotConnected
	}
	m.state = s
	m.updates++
	return nil
}

func (m *Mock) SetRumbleHandler(fn RumbleHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rumble = fn
}

func (m *Mock) Capabilities() Capabilities {
	return Capabilities{Rumble: true, Guide: true}
}

func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.state = protocol.Neutral
	return nil
}

// State returns the last state and the number of updates received.
func (m *Mock) State() (protocol.ControllerState, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.updates
}

// SimulateRumble behaves as if the host set the motors.
func (m *Mock) SimulateRumble(left, right uint16) {
	m.mu.Lock()
	fn := m.rumble
	m.mu.Unlock()
	if fn != nil {
		fn(left, right)
	}
}
