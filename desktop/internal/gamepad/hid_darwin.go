//go:build darwin && cgo

package gamepad

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hidsystem/IOHIDUserDevice.h>
#include <IOKit/hid/IOHIDDeviceKeys.h>
#include <mach/mach_time.h>
#include <stdint.h>
#include <stdlib.h>

// All CoreFoundation work stays in C: cgo's mapping of the bridged CF
// typedefs is inconsistent, so Go only sees an opaque handle and plain ints.

typedef struct {
	IOHIDUserDeviceRef dev;
} pp_device;

static pp_device *pp_device_create(const uint8_t *descriptor, size_t descriptor_len, const char *product) {
	CFMutableDictionaryRef props = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (props == NULL) {
		return NULL;
	}

	CFStringRef key = CFStringCreateWithCString(kCFAllocatorDefault,
		kIOHIDReportDescriptorKey, kCFStringEncodingUTF8);
	CFDataRef data = CFDataCreate(kCFAllocatorDefault, descriptor, (CFIndex)descriptor_len);
	CFStringRef product_key = CFStringCreateWithCString(kCFAllocatorDefault,
		kIOHIDProductKey, kCFStringEncodingUTF8);
	CFStringRef product_value = CFStringCreateWithCString(kCFAllocatorDefault,
		product, kCFStringEncodingUTF8);
	if (key == NULL || data == NULL || product_key == NULL || product_value == NULL) {
		if (key) CFRelease(key);
		if (data) CFRelease(data);
		if (product_key) CFRelease(product_key);
		if (product_value) CFRelease(product_value);
		CFRelease(props);
		return NULL;
	}
	CFDictionarySetValue(props, key, data);
	CFDictionarySetValue(props, product_key, product_value);

	pp_device *out = (pp_device *)calloc(1, sizeof(pp_device));
	if (out != NULL) {
		out->dev = IOHIDUserDeviceCreateWithProperties(kCFAllocatorDefault, props, 0);
		if (out->dev == NULL) {
			free(out);
			out = NULL;
		}
	}

	CFRelease(key);
	CFRelease(data);
	CFRelease(product_key);
	CFRelease(product_value);
	CFRelease(props);
	return out;
}

static int32_t pp_device_report(pp_device *d, const uint8_t *report, size_t len) {
	if (d == NULL || d->dev == NULL) {
		return (int32_t)0xE00002C0; // kIOReturnNotOpen
	}
	return (int32_t)IOHIDUserDeviceHandleReportWithTimeStamp(d->dev,
		mach_absolute_time(), report, (CFIndex)len);
}

static void pp_device_release(pp_device *d) {
	if (d == NULL) {
		return;
	}
	if (d->dev != NULL) {
		CFRelease(d->dev);
	}
	free(d);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/bhaavyasura7/pocketpad/desktop/internal/protocol"
)

func init() {
	Register("machid", func() (Gamepad, error) { return newMachid(), nil })
	platformDefault = "machid"
	autoCandidates = []string{"machid", "mackeys"}
}

// machid is the macOS virtual gamepad: an IOHIDUserDevice carrying the shared
// gamepad report. macOS has no public unprivileged API for this, so since
// macOS 11 creating the device needs either the restricted
// com.apple.developer.hid.virtual.device entitlement (Apple must grant it)
// or root; Connect reports exactly that when creation is refused.
//
// Rumble is not available for a generic HID pad on macOS.
type machid struct {
	mu     sync.Mutex
	dev    *C.pp_device
	rumble RumbleHandler
}

func newMachid() *machid { return &machid{} }

// Connect creates the virtual device.
func (m *machid) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev != nil {
		return nil
	}
	product := C.CString("PocketPad Virtual Gamepad")
	defer C.free(unsafe.Pointer(product))

	dev := C.pp_device_create(
		(*C.uint8_t)(unsafe.Pointer(&HIDDescriptor[0])),
		C.size_t(len(HIDDescriptor)),
		product,
	)
	if dev == nil {
		return fmt.Errorf("gamepad: macOS refused to create the virtual HID device. " +
			"Since macOS 11 this API requires the com.apple.developer.hid.virtual.device " +
			"entitlement (Apple must grant it) or root privileges; for a local build run " +
			"\"sudo pocketpad serve\". On macOS 26 use \"-backend mackeys\" to control the " +
			"Mac with keyboard and mouse instead, or \"-backend mock\" for diagnostics")
	}
	m.dev = dev
	return nil
}

// Update sends one report to the host.
func (m *machid) Update(s protocol.ControllerState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return ErrNotConnected
	}
	r := ToHIDReport(s)
	res := C.pp_device_report(m.dev,
		(*C.uint8_t)(unsafe.Pointer(&r[0])), C.size_t(len(r)))
	if res != 0 {
		return fmt.Errorf("gamepad: sending a HID report failed (IOReturn 0x%08x)", uint32(res))
	}
	return nil
}

// SetRumbleHandler is accepted for interface symmetry; a generic HID gamepad
// on macOS has no rumble channel, and Capabilities reports Rumble: false.
func (m *machid) SetRumbleHandler(fn RumbleHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rumble = fn
}

// Capabilities describes the backend.
func (m *machid) Capabilities() Capabilities {
	return Capabilities{Rumble: false, Guide: true, OSVisible: true}
}

// Close sends a neutral report and releases the device.
func (m *machid) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dev == nil {
		return nil
	}
	neutral := ToHIDReport(protocol.Neutral)
	C.pp_device_report(m.dev,
		(*C.uint8_t)(unsafe.Pointer(&neutral[0])), C.size_t(len(neutral)))
	C.pp_device_release(m.dev)
	m.dev = nil
	return nil
}
