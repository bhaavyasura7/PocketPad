// The native macOS window for `pocketpad app`: a QR code the phone scans,
// the pairing code, the LAN address and live connection status.
// Compiled by cgo as Objective-C; Go only calls plain C entry points.

#import <Cocoa/Cocoa.h>
#include <stdint.h>
#include <stdlib.h>

static NSWindow *gWindow = nil;
static NSTextField *gStatus = nil;

@interface PPAppDelegate : NSObject <NSApplicationDelegate>
@end

@implementation PPAppDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return YES;
}
@end

static PPAppDelegate *gDelegate = nil;

// Updates the status line from any thread.
void pp_app_set_status(const char *text) {
	NSString *value = [NSString stringWithUTF8String:(text ? text : "")];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gStatus != nil) {
			gStatus.stringValue = value;
		}
	});
}

// Builds the window and runs the application loop (blocks until quit).
void pp_app_launch(const char *title, const char *code, const char *address,
                   const uint8_t *png, size_t pngLen,
                   const uint8_t *logo, size_t logoLen) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		gDelegate = [[PPAppDelegate alloc] init];
		[app setDelegate:gDelegate];

		NSRect frame = NSMakeRect(0, 0, 440, 640);
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable;
		gWindow = [[NSWindow alloc] initWithContentRect:frame styleMask:style backing:NSBackingStoreBuffered defer:NO];
		gWindow.title = [NSString stringWithUTF8String:title];
		gWindow.releasedWhenClosed = NO;
		[gWindow center];
		NSView *content = gWindow.contentView;

		CGFloat y = 596;

		// Official logo beside the heading.
		if (logo != NULL && logoLen > 0) {
			NSImageView *logoView = [[NSImageView alloc] initWithFrame:NSMakeRect(24, y - 30, 64, 64)];
			logoView.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:logo length:logoLen]];
			logoView.imageScaling = NSImageScaleProportionallyUpOrDown;
			logoView.wantsLayer = YES;
			logoView.layer.cornerRadius = 14;
			logoView.layer.masksToBounds = YES;
			[content addSubview:logoView];
		}

		NSTextField *heading = [NSTextField labelWithString:@"PocketPad"];
		heading.font = [NSFont boldSystemFontOfSize:24];
		heading.frame = NSMakeRect(104, y, 312, 32);
		[content addSubview:heading];
		y -= 26;

		NSTextField *sub = [NSTextField labelWithString:@"Scan this with the PocketPad app on your phone — it fills everything in for you."];
		sub.font = [NSFont systemFontOfSize:12];
		sub.textColor = [NSColor secondaryLabelColor];
		sub.frame = NSMakeRect(104, y - 8, 312, 32);
		sub.maximumNumberOfLines = 2;
		sub.lineBreakMode = NSLineBreakByWordWrapping;
		[content addSubview:sub];
		y -= 44;

		// QR code: 300x300 centred.
		NSImageView *qr = [[NSImageView alloc] initWithFrame:NSMakeRect(70, y - 304, 300, 300)];
		NSData *data = [NSData dataWithBytes:png length:pngLen];
		NSImage *image = [[NSImage alloc] initWithData:data];
		qr.image = image;
		qr.imageScaling = NSImageScaleProportionallyUpOrDown;
		[qr setWantsLayer:YES];
		qr.layer.cornerRadius = 12;
		qr.layer.backgroundColor = [[NSColor whiteColor] CGColor];
		[content addSubview:qr];
		y -= 322;

		NSTextField *codeHelp = [NSTextField labelWithString:@"Or type this code in the app:"];
		codeHelp.font = [NSFont systemFontOfSize:12];
		codeHelp.textColor = [NSColor secondaryLabelColor];
		codeHelp.frame = NSMakeRect(24, y, 392, 16);
		[content addSubview:codeHelp];
		y -= 26;

		NSTextField *codeField = [NSTextField labelWithString:[NSString stringWithUTF8String:code]];
		codeField.font = [NSFont monospacedSystemFontOfSize:16 weight:NSFontWeightSemibold];
		codeField.selectable = YES;
		codeField.frame = NSMakeRect(24, y, 392, 22);
		[content addSubview:codeField];
		y -= 30;

		NSTextField *addrField = [NSTextField labelWithString:[NSString stringWithUTF8String:address]];
		addrField.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
		addrField.textColor = [NSColor secondaryLabelColor];
		addrField.selectable = YES;
		addrField.frame = NSMakeRect(24, y, 392, 18);
		[content addSubview:addrField];
		y -= 36;

		gStatus = [NSTextField labelWithString:@"Starting…"];
		gStatus.font = [NSFont systemFontOfSize:13];
		gStatus.maximumNumberOfLines = 3;
		gStatus.lineBreakMode = NSLineBreakByWordWrapping;
		gStatus.frame = NSMakeRect(24, y - 36, 392, 56);
		[content addSubview:gStatus];

		[gWindow makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];
		[app run];
	}
}
