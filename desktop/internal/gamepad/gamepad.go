// Package gamepad defines the interface between the session manager and
// the operating-system-visible virtual controller, and holds the backends.
package gamepad

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// Capabilities describes what a backend supports.
type Capabilities struct {
	// Rumble is true when the backend reports the game's motor output.
	Rumble bool
	// Guide is true when the Guide button reaches the host.
	Guide bool
	// OSVisible is true when the host sees a real game controller. Only
	// the mock backend reports false.
	OSVisible bool
}

// RumbleHandler receives motor strengths (0 = off, 0xFFFF = full) that the
// host sets on the virtual controller. It may be called from any goroutine
// and must not block.
type RumbleHandler func(left, right uint16)

// Gamepad is a virtual controller visible to the operating system.
//
// It extends the interface from the specification: SetRumble is replaced by
// SetRumbleHandler because rumble flows from the host to the phone, and
// Capabilities lets the session manager negotiate features.
type Gamepad interface {
	// Connect plugs the virtual controller in.
	Connect() error
	// Update reports a new controller state to the host.
	Update(state protocol.ControllerState) error
	// SetRumbleHandler registers the receiver of rumble output. Pass nil
	// to unregister.
	SetRumbleHandler(fn RumbleHandler)
	// Capabilities describes the backend.
	Capabilities() Capabilities
	// Close unplugs the controller. Close implies all inputs released.
	Close() error
}

// Factory creates a backend.
type Factory func() (Gamepad, error)

var (
	mu       sync.Mutex
	backends = map[string]Factory{}
	// platformDefault is the backend "auto" selects; set by platform files.
	platformDefault string
	// autoCandidates is the ordered list "auto" tries until one connects;
	// set by platform files. This is what makes a single `serve` command
	// work from macOS 15 to macOS 26 without flags.
	autoCandidates []string
)

// Register makes a backend available under name.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	backends[name] = f
}

// Names returns the registered backend names.
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, 0, len(backends))
	for n := range backends {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ErrNoPlatformBackend means this OS has no virtual controller backend.
var ErrNoPlatformBackend = errors.New("gamepad: no virtual controller backend for this operating system (use -backend mock for diagnostics)")

// Open creates the named backend. "auto" selects the platform backend.
func Open(name string) (Gamepad, error) {
	mu.Lock()
	if name == "auto" {
		name = platformDefault
	}
	f, ok := backends[name]
	mu.Unlock()
	if name == "" {
		return nil, ErrNoPlatformBackend
	}
	if !ok {
		return nil, fmt.Errorf("gamepad: unknown backend %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return f()
}

// ConnectBest opens and connects a backend. "auto" tries every platform
// candidate in order and returns the first that connects, so on macOS it
// can fall back from the virtual-HID backend to keyboard/mouse control
// without the user knowing anything about entitlements. The returned name
// is the backend that actually started.
func ConnectBest(name string) (Gamepad, string, error) {
	if name != "auto" && name != "" {
		pad, err := Open(name)
		if err != nil {
			return nil, name, err
		}
		if err := pad.Connect(); err != nil {
			return nil, name, err
		}
		return pad, name, nil
	}

	candidates := append([]string(nil), autoCandidates...)
	if len(candidates) == 0 && platformDefault != "" {
		candidates = []string{platformDefault}
	}
	if len(candidates) == 0 {
		return nil, "", ErrNoPlatformBackend
	}

	var attempts []string
	for _, c := range candidates {
		pad, err := Open(c)
		if err == nil {
			err = pad.Connect()
		}
		if err == nil {
			return pad, c, nil
		}
		attempts = append(attempts, fmt.Sprintf("%s: %v", c, err))
	}
	return nil, "", fmt.Errorf("no gamepad backend could start:\n  %s", strings.Join(attempts, "\n  "))
}
