//go:build windows

package gamepad

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

// The ViGEm backend creates a virtual Xbox 360 controller through the ViGEmBus
// driver, which Windows and XInput games see as a real pad. It needs:
//
//   - ViGEmBus installed (https://github.com/nefarius/ViGEmBus/releases); the
//     user installs it explicitly, PocketPad never installs drivers itself.
//   - ViGEmClient.dll (x64 or ARM64, matching this binary) next to
//     pocketpad.exe or on PATH.
//
// ViGEmBus is retired upstream but still works on Windows 10 and 11; the
// backend sits behind the Gamepad interface so it can be replaced.
func init() {
	Register("vigem", func() (Gamepad, error) { return newViGEm() })
	platformDefault = "vigem"
	autoCandidates = []string{"vigem"}
}

const vigemErrorNone = 0x20000000

var (
	vigemDLL          = syscall.NewLazyDLL("ViGEmClient.dll")
	procAlloc         = vigemDLL.NewProc("vigem_alloc")
	procFree          = vigemDLL.NewProc("vigem_free")
	procConnect       = vigemDLL.NewProc("vigem_connect")
	procDisconnect    = vigemDLL.NewProc("vigem_disconnect")
	procX360Alloc     = vigemDLL.NewProc("vigem_target_x360_alloc")
	procTargetFree    = vigemDLL.NewProc("vigem_target_free")
	procTargetAdd     = vigemDLL.NewProc("vigem_target_add")
	procTargetRemove  = vigemDLL.NewProc("vigem_target_remove")
	procX360Update    = vigemDLL.NewProc("vigem_target_x360_update")
	procX360RegNotify = vigemDLL.NewProc("vigem_target_x360_register_notification")
	procX360UnNotify  = vigemDLL.NewProc("vigem_target_x360_unregister_notification")
)

var vigemErrors = map[uintptr]string{
	0xE0000001: "ViGEmBus driver not found: install ViGEmBus (https://github.com/nefarius/ViGEmBus/releases)",
	0xE0000002: "no free virtual controller slot",
	0xE0000004: "removal failed",
	0xE0000005: "already connected",
	0xE0000006: "target uninitialized",
	0xE0000007: "target not plugged in",
	0xE0000008: "ViGEmBus version mismatch: update ViGEmBus",
	0xE0000009: "ViGEmBus access failed (is another program holding it?)",
}

func vigemErr(op string, r uintptr) error {
	if uint32(r) == vigemErrorNone {
		return nil
	}
	if msg, ok := vigemErrors[uintptr(uint32(r))]; ok {
		return fmt.Errorf("vigem %s: %s", op, msg)
	}
	return fmt.Errorf("vigem %s: error 0x%08x", op, uint32(r))
}

type vigem struct {
	mu        sync.Mutex
	client    uintptr
	target    uintptr
	connected bool
	rumble    RumbleHandler
}

// One callback for the process: syscall.NewCallback slots are never freed.
var (
	notifyOnce sync.Once
	notifyCB   uintptr
	notifyMu   sync.Mutex
	notifyPads = map[uintptr]*vigem{}
)

func newViGEm() (*vigem, error) {
	if err := vigemDLL.Load(); err != nil {
		return nil, fmt.Errorf("vigem: cannot load ViGEmClient.dll (place it next to pocketpad.exe): %w", err)
	}
	notifyOnce.Do(func() {
		// void (*)(PVIGEM_CLIENT, PVIGEM_TARGET, UCHAR large, UCHAR small, UCHAR led, LPVOID user)
		notifyCB = syscall.NewCallback(func(client, target, large, small, led, user uintptr) uintptr {
			notifyMu.Lock()
			p := notifyPads[target]
			notifyMu.Unlock()
			if p != nil {
				p.mu.Lock()
				fn := p.rumble
				p.mu.Unlock()
				if fn != nil {
					fn(motor8To16(uint8(large)), motor8To16(uint8(small)))
				}
			}
			return 0
		})
	})
	return &vigem{}, nil
}

func (v *vigem) Connect() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.connected {
		return nil
	}
	client, _, _ := procAlloc.Call()
	if client == 0 {
		return errors.New("vigem: allocation failed")
	}
	if r, _, _ := procConnect.Call(client); vigemErr("connect", r) != nil {
		procFree.Call(client)
		return vigemErr("connect", r)
	}
	target, _, _ := procX360Alloc.Call()
	if target == 0 {
		procDisconnect.Call(client)
		procFree.Call(client)
		return errors.New("vigem: target allocation failed")
	}
	if r, _, _ := procTargetAdd.Call(client, target); vigemErr("plug in", r) != nil {
		procTargetFree.Call(target)
		procDisconnect.Call(client)
		procFree.Call(client)
		return vigemErr("plug in", r)
	}
	notifyMu.Lock()
	notifyPads[target] = v
	notifyMu.Unlock()
	if r, _, _ := procX360RegNotify.Call(client, target, notifyCB, 0); vigemErr("register rumble", r) != nil {
		// Input still works without rumble.
		notifyMu.Lock()
		delete(notifyPads, target)
		notifyMu.Unlock()
	}
	v.client, v.target, v.connected = client, target, true
	return nil
}

func (v *vigem) Update(s protocol.ControllerState) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.connected {
		return ErrNotConnected
	}
	rep := ToXUSB(s)
	b := rep.Bytes()
	var r uintptr
	switch runtime.GOARCH {
	case "arm64":
		// AArch64 passes a 12-byte struct by value in two registers.
		lo := *(*uint64)(unsafe.Pointer(&b[0]))
		hi := uint64(*(*uint32)(unsafe.Pointer(&b[8])))
		r, _, _ = procX360Update.Call(v.client, v.target, uintptr(lo), uintptr(hi))
	default:
		// x64 passes structs larger than 8 bytes by reference to a copy.
		r, _, _ = procX360Update.Call(v.client, v.target, uintptr(unsafe.Pointer(&b[0])))
	}
	runtime.KeepAlive(&b)
	return vigemErr("update", r)
}

func (v *vigem) SetRumbleHandler(fn RumbleHandler) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.rumble = fn
}

func (v *vigem) Capabilities() Capabilities {
	return Capabilities{Rumble: true, Guide: true, OSVisible: true}
}

func (v *vigem) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.connected {
		return nil
	}
	b := ToXUSB(protocol.Neutral).Bytes()
	if runtime.GOARCH != "arm64" {
		procX360Update.Call(v.client, v.target, uintptr(unsafe.Pointer(&b[0])))
	}
	procX360UnNotify.Call(v.target)
	notifyMu.Lock()
	delete(notifyPads, v.target)
	notifyMu.Unlock()
	procTargetRemove.Call(v.client, v.target)
	procTargetFree.Call(v.target)
	procDisconnect.Call(v.client)
	procFree.Call(v.client)
	v.connected = false
	return nil
}
