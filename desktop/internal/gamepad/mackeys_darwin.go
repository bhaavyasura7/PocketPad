//go:build darwin && cgo

package gamepad

/*
#cgo LDFLAGS: -framework ApplicationServices

#include <ApplicationServices/ApplicationServices.h>

// All Core Foundation work stays in C; Go only sees ints and calls.

static int pp_ax_trusted(int prompt) {
	CFStringRef key = kAXTrustedCheckOptionPrompt;
	CFBooleanRef value = prompt ? kCFBooleanTrue : kCFBooleanFalse;
	CFDictionaryRef options = CFDictionaryCreate(NULL,
		(const void **)&key, (const void **)&value, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	Boolean trusted = AXIsProcessTrustedWithOptions(options);
	CFRelease(options);
	return trusted ? 1 : 0;
}

static void pp_key(int keycode, int down) {
	CGEventRef ev = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keycode, down ? true : false);
	if (ev != NULL) {
		CGEventPost(kCGHIDEventTap, ev);
		CFRelease(ev);
	}
}

static void pp_mouse_location(double *x, double *y) {
	CGEventRef ev = CGEventCreate(NULL);
	CGPoint p = CGEventGetLocation(ev);
	CFRelease(ev);
	*x = p.x;
	*y = p.y;
}

// pp_mouse_move_by moves the pointer by a delta, clamped to the union of the
// active displays so the cursor can never run off-screen when a stick is
// held against an edge.
static void pp_mouse_move_by(double dx, double dy) {
	double x, y;
	pp_mouse_location(&x, &y);

	uint32_t count = 0;
	if (CGGetActiveDisplayList(0, NULL, &count) == kCGErrorSuccess && count > 0 && count <= 16) {
		CGDirectDisplayID displays[16];
		if (CGGetActiveDisplayList(16, displays, &count) == kCGErrorSuccess) {
			CGRect bounds = CGRectNull;
			for (uint32_t i = 0; i < count; i++) {
				bounds = CGRectUnion(bounds, CGDisplayBounds(displays[i]));
			}
			if (!CGRectIsNull(bounds)) {
				double maxX = bounds.origin.x + bounds.size.width - 1;
				double maxY = bounds.origin.y + bounds.size.height - 1;
				x += dx;
				y += dy;
				if (x < bounds.origin.x) x = bounds.origin.x;
				if (y < bounds.origin.y) y = bounds.origin.y;
				if (x > maxX) x = maxX;
				if (y > maxY) y = maxY;
			} else {
				x += dx;
				y += dy;
			}
		}
	} else {
		x += dx;
		y += dy;
	}

	CGPoint p = CGPointMake(x, y);
	CGEventRef ev = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, p, kCGMouseButtonLeft);
	if (ev != NULL) {
		CGEventPost(kCGHIDEventTap, ev);
		CFRelease(ev);
	}
}

static void pp_mouse_button(int right, int down) {
	double x, y;
	pp_mouse_location(&x, &y);
	CGEventType type;
	CGMouseButton button;
	if (right) {
		type = down ? kCGEventRightMouseDown : kCGEventRightMouseUp;
		button = kCGMouseButtonRight;
	} else {
		type = down ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
		button = kCGMouseButtonLeft;
	}
	CGPoint p = CGPointMake(x, y);
	CGEventRef ev = CGEventCreateMouseEvent(NULL, type, p, button);
	if (ev != NULL) {
		CGEventPost(kCGHIDEventTap, ev);
		CFRelease(ev);
	}
}

// pp_scroll turns the wheel: positive lines scroll up (map zoom in,
// previous weapon), negative scroll down.
static void pp_scroll(int32_t lines) {
	CGEventRef ev = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 1, lines);
	if (ev != NULL) {
		CGEventPost(kCGHIDEventTap, ev);
		CFRelease(ev);
	}
}
*/
import "C"

import (
	"math"
	"sync"
	"time"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func init() {
	Register("mackeys", func() (Gamepad, error) { return newMacKeys(), nil })
}

// mouseSpeedPxPerSecond is the pointer speed at full right-stick deflection.
const mouseSpeedPxPerSecond = 700

// macKeys drives the Mac with synthetic keyboard and mouse events. It needs
// the user's Accessibility approval; until it is granted the backend stays
// up and simply waits, so enabling the toggle in System Settings starts
// control immediately without a restart.
type macKeys struct {
	mu             sync.Mutex
	held           map[int]bool
	ltDown         bool
	rtDown         bool
	last           time.Time
	lastTrustCheck time.Time
	trusted        bool
	started        bool
	rumble         RumbleHandler

	// Sub-pixel pointer remainder: the curved response can ask for less than
	// one pixel per tick, and without carrying the remainder the cursor would
	// stall at slow speeds.
	fracX, fracY float64

	leftDown  bool
	rightDown bool
	wheelHeld map[int]bool
}

func newMacKeys() *macKeys {
	return &macKeys{held: map[int]bool{}, wheelHeld: map[int]bool{}}
}

// MacKeysTrusted reports whether this process may post synthetic events.
func MacKeysTrusted() bool { return C.pp_ax_trusted(C.int(0)) != 0 }

// Connect never fails: without the permission the system prompt is shown
// and the backend keeps polling until the user enables it.
func (m *macKeys) Connect() error {
	if C.pp_ax_trusted(C.int(1)) != 0 {
		m.trusted = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = time.Now()
	m.started = true
	return nil
}

// Update diffs the desired key set and pointer against what is held and posts
// only the changes, so keys never stick and events stay cheap.
func (m *macKeys) Update(s protocol.ControllerState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return ErrNotConnected
	}
	if !m.trusted {
		// Poll a few times a second until the toggle is flipped.
		now := time.Now()
		if now.Sub(m.lastTrustCheck) > 500*time.Millisecond {
			m.lastTrustCheck = now
			if C.pp_ax_trusted(C.int(0)) != 0 {
				m.trusted = true
			}
		}
		if !m.trusted {
			return nil
		}
	}
	p := currentProfile()

	// Keys: diff the desired set against what is held.
	want := desiredKeys(s, p)
	for key := range m.held {
		if !want[key] {
			C.pp_key(C.int(key), C.int(0))
			delete(m.held, key)
		}
	}
	for key := range want {
		if !m.held[key] {
			C.pp_key(C.int(key), C.int(1))
			m.held[key] = true
		}
	}

	// Mouse buttons: any control the profile maps to a click holds it down
	// while pressed.
	ml, mr := desiredMouse(s, p)
	if ml != m.leftDown {
		C.pp_mouse_button(C.int(0), C.int(boolInt(ml)))
		m.leftDown = ml
	}
	if mr != m.rightDown {
		C.pp_mouse_button(C.int(1), C.int(boolInt(mr)))
		m.rightDown = mr
	}

	// Wheel actions fire one notch per press edge.
	active := activeWheels(s, p)
	for src, dir := range active {
		if !m.wheelHeld[src] {
			C.pp_scroll(C.int32_t(dir))
		}
	}
	m.wheelHeld = make(map[int]bool, len(active))
	for src := range active {
		m.wheelHeld[src] = true
	}

	// Right stick moves the pointer from wherever it is now, so a physical
	// mouse stays usable at the same time.
	now := time.Now()
	dt := now.Sub(m.last).Seconds()
	if dt > 0.05 {
		dt = 0.05
	}
	m.last = now
	if p.RightStickMouse && (s.RightX != 0 || s.RightY != 0) {
		speed := float32(mouseSpeedPxPerSecond)
		if p.MouseSpeedPxPerSecond > 0 {
			speed = float32(p.MouseSpeedPxPerSecond)
		}
		dx, dy := mouseDelta(s.RightX, s.RightY, float32(dt), speed)
		m.fracX += dx
		m.fracY += dy
		ix, iy := math.Trunc(m.fracX), math.Trunc(m.fracY)
		if ix != 0 || iy != 0 {
			m.fracX -= ix
			m.fracY -= iy
			C.pp_mouse_move_by(C.double(ix), C.double(iy))
		}
	}
	return nil
}

func (m *macKeys) SetRumbleHandler(fn RumbleHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rumble = fn
}

// Capabilities describes the backend. Keyboard/mouse emulation is visible to
// the system but is not a game controller.
func (m *macKeys) Capabilities() Capabilities {
	return Capabilities{Rumble: false, Guide: true, OSVisible: true}
}

// Close releases every held key and button.
func (m *macKeys) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return nil
	}
	for key := range m.held {
		C.pp_key(C.int(key), C.int(0))
	}
	m.held = map[int]bool{}
	if m.leftDown {
		C.pp_mouse_button(C.int(0), C.int(0))
		m.leftDown = false
	}
	if m.rightDown {
		C.pp_mouse_button(C.int(1), C.int(0))
		m.rightDown = false
	}
	m.wheelHeld = map[int]bool{}
	m.started = false
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
