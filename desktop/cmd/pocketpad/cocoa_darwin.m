// The native macOS window for `pocketpad app`: a QR code the phone scans,
// the pairing code, live connection status, the keyboard/mouse profile and
// a mapping editor. Built from standard AppKit controls with Auto Layout,
// so it follows the system appearance (light or dark) and accent colour.
// Compiled by cgo as Objective-C; Go only calls plain C entry points.

#import <Cocoa/Cocoa.h>
#include <stdint.h>
#include <stdlib.h>

// Implemented in Go (cgo //export in app_darwin.go). Returned strings are
// malloc'd and must be freed by the caller.
extern char *ppGoCurrentMappingJSON(void);
extern char *ppGoSaveMapping(const char *json);
extern char *ppGoSetProfile(const char *name);
extern char *ppGoCurrentProfileName(void);

// Status states sent by pp_app_set_status.
enum { PPStateWaiting = 0, PPStateConnected = 1, PPStatePermission = 2 };

// --- helpers ---------------------------------------------------------------

/** Top-down coordinates, for the editor's scroll document. */
@interface PPFlippedView : NSView
@end

@implementation PPFlippedView
- (BOOL)isFlipped {
	return YES;
}
@end

static NSTextField *PPText(NSString *text, NSFont *font, NSColor *color) {
	NSTextField *label = [NSTextField labelWithString:text ?: @""];
	label.font = font;
	label.textColor = color;
	label.translatesAutoresizingMaskIntoConstraints = NO;
	return label;
}

/** A wrapping, centred label limited to a preferred width. */
static NSTextField *PPWrapping(NSString *text, NSFont *font, NSColor *color, CGFloat width) {
	NSTextField *label = [NSTextField wrappingLabelWithString:text ?: @""];
	label.font = font;
	label.textColor = color;
	label.alignment = NSTextAlignmentCenter;
	label.preferredMaxLayoutWidth = width;
	label.translatesAutoresizingMaskIntoConstraints = NO;
	return label;
}

/** A section background that tracks the system appearance. */
static NSBox *PPGroup(void) {
	NSBox *box = [[NSBox alloc] init];
	box.boxType = NSBoxCustom;
	box.titlePosition = NSNoTitle;
	box.fillColor = [NSColor controlBackgroundColor];
	box.borderColor = [NSColor separatorColor];
	box.borderWidth = 1;
	box.cornerRadius = 8;
	box.contentViewMargins = NSZeroSize;
	return box;
}

static void PPAddItem(NSPopUpButton *popup, NSString *title, NSString *value) {
	[popup addItemWithTitle:title];
	popup.lastItem.representedObject = value;
}

static BOOL PPSelectValue(NSPopUpButton *popup, NSString *value) {
	for (NSMenuItem *item in popup.itemArray) {
		if ([item.representedObject isEqual:value]) {
			[popup selectItem:item];
			return YES;
		}
	}
	return NO;
}

static NSString *PPSelectedValue(NSPopUpButton *popup) {
	id value = popup.selectedItem.representedObject;
	return [value isKindOfClass:[NSString class]] ? value : @"none";
}

static NSArray *PPProfiles(void) {
	return @[@[@"general", @"General"], @[@"racing", @"Racing"], @[@"gta", @"GTA"], @[@"custom", @"Custom"]];
}

static NSString *PPProfileTitle(NSString *name) {
	for (NSArray *p in PPProfiles()) {
		if ([p[0] isEqualToString:name]) {
			return p[1];
		}
	}
	return name;
}

static NSPopUpButton *PPMakeProfilePopup(void) {
	NSPopUpButton *popup = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
	for (NSArray *p in PPProfiles()) {
		PPAddItem(popup, p[1], p[0]);
	}
	return popup;
}

static NSString *PPCurrentProfile(void) {
	char *c = ppGoCurrentProfileName();
	NSString *name = [NSString stringWithUTF8String:(c != NULL ? c : "general")];
	if (c != NULL) {
		free((void *)c);
	}
	return name;
}

// --- state -----------------------------------------------------------------

static NSWindow *gWindow = nil;
static NSImageView *gStatusDot = nil;
static NSTextField *gStatusTitle = nil;
static NSTextField *gStatusDetail = nil;
static NSTextField *gStats = nil;
static NSView *gPermissionBanner = nil;
static NSButton *gCopyButton = nil;
static NSPopUpButton *gMainProfilePopup = nil;
static NSString *gCodeString = nil;
static NSString *gModeTitle = nil;
static BOOL gKeyboardMode = NO;

static NSWindow *gEditorWindow = nil;
static NSPopUpButton *gEditorProfilePopup = nil;
static NSTextField *gEditorStatus = nil;
static NSMutableArray *gRowKeys = nil;
static NSMutableArray *gRowPopups = nil;
static NSDictionary *gJSON = nil;

void pp_app_show_editor(const char *json);
static void PPFillRowsFromJSON(void);
static NSDictionary *PPBuildDoc(void);
static void PPEditorStatus(NSString *text);

/** Reflects the active profile in the subtitle and both profile menus. */
static void PPShowProfile(NSString *name) {
	if (gKeyboardMode) {
		gWindow.subtitle = [NSString stringWithFormat:@"%@ · %@ profile", gModeTitle, PPProfileTitle(name)];
	}
	if (gMainProfilePopup != nil) {
		PPSelectValue(gMainProfilePopup, name);
	}
	if (gEditorProfilePopup != nil) {
		PPSelectValue(gEditorProfilePopup, name);
	}
}

// --- delegate --------------------------------------------------------------

@interface PPAppDelegate : NSObject <NSApplicationDelegate>
@end

@implementation PPAppDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return YES;
}

- (void)showMappings:(id)sender {
	pp_app_show_editor(ppGoCurrentMappingJSON());
}

- (void)copyCode:(id)sender {
	if (gCodeString == nil) {
		return;
	}
	NSPasteboard *pb = [NSPasteboard generalPasteboard];
	[pb clearContents];
	[pb setString:gCodeString forType:NSPasteboardTypeString];
	gCopyButton.title = @"Copied";
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.5 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		gCopyButton.title = @"Copy";
	});
}

- (void)openAccessibilitySettings:(id)sender {
	NSURL *url = [NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"];
	[[NSWorkspace sharedWorkspace] openURL:url];
}

- (void)profileChanged:(id)sender {
	NSString *name = PPSelectedValue((NSPopUpButton *)sender);
	NSString *previous = PPCurrentProfile();
	char *err = ppGoSetProfile(name.UTF8String);
	if (err != NULL) {
		free((void *)err);
		PPShowProfile(previous);
		if ([name isEqualToString:@"custom"]) {
			// No custom mapping exists yet: open the editor to create one.
			pp_app_show_editor(ppGoCurrentMappingJSON());
			PPEditorStatus(@"There is no custom mapping yet. Choose keys below, then click Save.");
		} else {
			PPEditorStatus([NSString stringWithFormat:@"Could not switch to the %@ profile.", PPProfileTitle(name)]);
		}
		return;
	}
	PPShowProfile(name);
	if (gEditorWindow != nil && gEditorWindow.visible) {
		pp_app_show_editor(ppGoCurrentMappingJSON());
	}
}

- (void)saveMapping:(id)sender {
	NSData *data = [NSJSONSerialization dataWithJSONObject:PPBuildDoc() options:NSJSONWritingPrettyPrinted error:NULL];
	if (data == nil) {
		PPEditorStatus(@"The mapping could not be encoded.");
		return;
	}
	NSString *text = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
	char *err = ppGoSaveMapping(text.UTF8String);
	if (err != NULL) {
		NSString *message = [NSString stringWithUTF8String:err];
		free((void *)err);
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = @"The mapping could not be saved.";
		alert.informativeText = message;
		[alert beginSheetModalForWindow:gEditorWindow completionHandler:nil];
		return;
	}
	PPShowProfile(@"custom");
	PPEditorStatus(@"Saved. The custom profile is now active.");
}

- (void)revertMapping:(id)sender {
	pp_app_show_editor(ppGoCurrentMappingJSON());
	PPEditorStatus(@"Reverted to the active mapping.");
}

- (void)closeEditor:(id)sender {
	[gEditorWindow orderOut:nil];
}
@end

static PPAppDelegate *gDelegate = nil;

// --- menu bar --------------------------------------------------------------

static void PPInstallMenu(void) {
	NSMenu *main = [[NSMenu alloc] init];

	NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"PocketPad"];
	[appMenu addItemWithTitle:@"About PocketPad" action:@selector(orderFrontStandardAboutPanel:) keyEquivalent:@""];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:@"Hide PocketPad" action:@selector(hide:) keyEquivalent:@"h"];
	NSMenuItem *hideOthers = [appMenu addItemWithTitle:@"Hide Others" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
	hideOthers.keyEquivalentModifierMask = NSEventModifierFlagOption | NSEventModifierFlagCommand;
	[appMenu addItemWithTitle:@"Show All" action:@selector(unhideAllApplications:) keyEquivalent:@""];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:@"Quit PocketPad" action:@selector(terminate:) keyEquivalent:@"q"];
	[main addItemWithTitle:@"" action:nil keyEquivalent:@""].submenu = appMenu;

	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	[main addItemWithTitle:@"Edit" action:nil keyEquivalent:@""].submenu = edit;

	NSMenu *window = [[NSMenu alloc] initWithTitle:@"Window"];
	[window addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
	[window addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];
	[main addItemWithTitle:@"Window" action:nil keyEquivalent:@""].submenu = window;

	NSApp.mainMenu = main;
	NSApp.windowsMenu = window;
}

// --- status ----------------------------------------------------------------

// Applies a status on the main thread.
static void PPApplyStatus(int state, NSString *t, NSString *d, NSString *s) {
	if (gStatusTitle == nil) {
		return;
	}
	gStatusDot.contentTintColor = (state == PPStateConnected) ? [NSColor systemGreenColor] : [NSColor systemOrangeColor];
	gStatusTitle.stringValue = t;
	gStatusDetail.stringValue = d;
	gStats.stringValue = s;
	gStats.hidden = (s.length == 0);
	gPermissionBanner.hidden = (state != PPStatePermission);
}

// Updates the status area from any thread.
void pp_app_set_status(int state, const char *title, const char *detail, const char *stats) {
	NSString *t = [NSString stringWithUTF8String:(title ? title : "")];
	NSString *d = [NSString stringWithUTF8String:(detail ? detail : "")];
	NSString *s = [NSString stringWithUTF8String:(stats ? stats : "")];
	dispatch_async(dispatch_get_main_queue(), ^{
		PPApplyStatus(state, t, d, s);
	});
}

// --- snapshots ---------------------------------------------------------------

// With POCKETPAD_SNAPSHOT_DIR set, the app renders its windows to PNG files
// in light and dark appearance and quits. Used by CI to show the UI.

static void PPWriteSnapshot(NSWindow *window, NSString *dir, NSString *name, NSString *appearance) {
	window.appearance = [NSAppearance appearanceNamed:appearance];
	NSView *view = window.contentView.superview ?: window.contentView; // include the title bar
	[window.contentView layoutSubtreeIfNeeded];
	[view setNeedsDisplay:YES];
	[window displayIfNeeded];
	NSBitmapImageRep *rep = [view bitmapImageRepForCachingDisplayInRect:view.bounds];
	[view cacheDisplayInRect:view.bounds toBitmapImageRep:rep];
	NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
	NSString *suffix = [appearance isEqualToString:NSAppearanceNameDarkAqua] ? @"dark" : @"light";
	[png writeToFile:[dir stringByAppendingPathComponent:[NSString stringWithFormat:@"%@-%@.png", name, suffix]] atomically:YES];
}

static void PPWriteBoth(NSWindow *window, NSString *dir, NSString *name) {
	PPWriteSnapshot(window, dir, name, NSAppearanceNameAqua);
	PPWriteSnapshot(window, dir, name, NSAppearanceNameDarkAqua);
}

static void PPRunSnapshots(NSString *dir) {
	[[NSFileManager defaultManager] createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:NULL];
	PPApplyStatus(PPStateWaiting, @"Waiting for your phone", @"Open PocketPad on your phone, on the same Wi-Fi network.", @"");
	PPWriteBoth(gWindow, dir, @"1-waiting");
	PPApplyStatus(PPStateConnected, @"Connected to Pixel 8", @"4.1 ms latency · Phone battery 80%",
	              @"250 updates/s · 0 lost · 0 late");
	PPWriteBoth(gWindow, dir, @"2-connected");
	PPApplyStatus(PPStatePermission, @"Permission needed",
	              @"Allow PocketPad under Accessibility in System Settings to control this Mac.", @"");
	PPWriteBoth(gWindow, dir, @"3-permission");
	if (gKeyboardMode) {
		pp_app_show_editor(ppGoCurrentMappingJSON());
		PPWriteBoth(gEditorWindow, dir, @"4-editor");
	}
	[NSApp terminate:nil];
}

// --- main window -----------------------------------------------------------

static NSView *PPPermissionBanner(CGFloat width) {
	NSBox *box = PPGroup();
	box.fillColor = [[NSColor systemOrangeColor] colorWithAlphaComponent:0.12];
	box.borderColor = [[NSColor systemOrangeColor] colorWithAlphaComponent:0.35];
	box.translatesAutoresizingMaskIntoConstraints = NO;

	NSImageView *icon = [NSImageView imageViewWithImage:[NSImage imageWithSystemSymbolName:@"exclamationmark.triangle.fill" accessibilityDescription:@"Warning"]];
	icon.contentTintColor = [NSColor systemOrangeColor];
	NSTextField *text = [NSTextField wrappingLabelWithString:@"PocketPad needs Accessibility permission to control this Mac."];
	text.font = [NSFont systemFontOfSize:12];
	text.preferredMaxLayoutWidth = width - 180;
	NSButton *open = [NSButton buttonWithTitle:@"Open Settings" target:gDelegate action:@selector(openAccessibilitySettings:)];
	open.controlSize = NSControlSizeSmall;

	NSStackView *row = [NSStackView stackViewWithViews:@[icon, text, open]];
	row.orientation = NSUserInterfaceLayoutOrientationHorizontal;
	row.alignment = NSLayoutAttributeCenterY;
	row.spacing = 8;
	row.edgeInsets = NSEdgeInsetsMake(10, 12, 10, 12);
	row.translatesAutoresizingMaskIntoConstraints = NO;
	[text setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
	[box.contentView addSubview:row];
	[NSLayoutConstraint activateConstraints:@[
		[row.leadingAnchor constraintEqualToAnchor:box.contentView.leadingAnchor],
		[row.trailingAnchor constraintEqualToAnchor:box.contentView.trailingAnchor],
		[row.topAnchor constraintEqualToAnchor:box.contentView.topAnchor],
		[row.bottomAnchor constraintEqualToAnchor:box.contentView.bottomAnchor],
		[box.widthAnchor constraintEqualToConstant:width],
	]];
	box.hidden = YES;
	return box;
}

// Builds the window and runs the application loop (blocks until quit).
void pp_app_launch(const char *modeTitle, const char *code, const char *address,
                   const uint8_t *png, size_t pngLen,
                   const uint8_t *logo, size_t logoLen,
                   int keyboardMode) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		gDelegate = [[PPAppDelegate alloc] init];
		[app setDelegate:gDelegate];
		if (logo != NULL && logoLen > 0) {
			// Dock icon and About panel when run outside the app bundle.
			app.applicationIconImage = [[NSImage alloc] initWithData:[NSData dataWithBytes:logo length:logoLen]];
		}
		PPInstallMenu();

		gCodeString = [NSString stringWithUTF8String:code];
		gModeTitle = [NSString stringWithUTF8String:modeTitle];
		gKeyboardMode = keyboardMode != 0;
		const CGFloat width = 380;

		gWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 440, 600)
		                                      styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable
		                                        backing:NSBackingStoreBuffered
		                                          defer:NO];
		gWindow.title = @"PocketPad";
		gWindow.subtitle = gModeTitle;
		gWindow.releasedWhenClosed = NO;

		// Status: coloured dot, headline and one line of detail.
		NSImage *dotImage = [NSImage imageWithSystemSymbolName:@"circle.fill" accessibilityDescription:@"Status"];
		gStatusDot = [NSImageView imageViewWithImage:dotImage];
		gStatusDot.symbolConfiguration = [NSImageSymbolConfiguration configurationWithPointSize:9 weight:NSFontWeightRegular];
		gStatusDot.contentTintColor = [NSColor systemOrangeColor];
		gStatusTitle = PPText(@"Starting…", [NSFont systemFontOfSize:15 weight:NSFontWeightSemibold], [NSColor labelColor]);
		NSStackView *headline = [NSStackView stackViewWithViews:@[gStatusDot, gStatusTitle]];
		headline.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		headline.alignment = NSLayoutAttributeCenterY;
		headline.spacing = 6;
		gStatusDetail = PPWrapping(@"", [NSFont systemFontOfSize:12], [NSColor secondaryLabelColor], width);

		gPermissionBanner = PPPermissionBanner(width);

		// QR code on white, as scanners expect.
		NSBox *qrBox = PPGroup();
		qrBox.fillColor = [NSColor whiteColor];
		qrBox.cornerRadius = 12;
		qrBox.translatesAutoresizingMaskIntoConstraints = NO;
		NSImageView *qr = [[NSImageView alloc] init];
		qr.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:pngLen]];
		qr.imageScaling = NSImageScaleProportionallyUpOrDown;
		qr.translatesAutoresizingMaskIntoConstraints = NO;
		[qrBox.contentView addSubview:qr];
		[NSLayoutConstraint activateConstraints:@[
			[qrBox.widthAnchor constraintEqualToConstant:220],
			[qrBox.heightAnchor constraintEqualToConstant:220],
			[qr.centerXAnchor constraintEqualToAnchor:qrBox.contentView.centerXAnchor],
			[qr.centerYAnchor constraintEqualToAnchor:qrBox.contentView.centerYAnchor],
			[qr.widthAnchor constraintEqualToConstant:196],
			[qr.heightAnchor constraintEqualToConstant:196],
		]];

		NSTextField *scanHelp = PPWrapping(@"Scan this code with the PocketPad app on your phone, or enter the pairing code.",
		                                   [NSFont systemFontOfSize:12], [NSColor secondaryLabelColor], width);

		// Pairing code with Copy.
		NSTextField *codeField = PPText(gCodeString, [NSFont monospacedSystemFontOfSize:16 weight:NSFontWeightMedium], [NSColor labelColor]);
		codeField.selectable = YES;
		gCopyButton = [NSButton buttonWithTitle:@"Copy" target:gDelegate action:@selector(copyCode:)];
		NSStackView *codeRow = [NSStackView stackViewWithViews:@[codeField, gCopyButton]];
		codeRow.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		codeRow.alignment = NSLayoutAttributeCenterY;
		codeRow.spacing = 10;

		NSTextField *addrField = PPText([NSString stringWithFormat:@"Address: %s", address],
		                                [NSFont monospacedDigitSystemFontOfSize:12 weight:NSFontWeightRegular], [NSColor secondaryLabelColor]);
		addrField.selectable = YES;

		gStats = PPWrapping(@"", [NSFont monospacedDigitSystemFontOfSize:11 weight:NSFontWeightRegular], [NSColor tertiaryLabelColor], width);
		gStats.hidden = YES;

		NSMutableArray *views = [NSMutableArray arrayWithArray:@[headline, gStatusDetail, gPermissionBanner, qrBox, scanHelp, codeRow, addrField, gStats]];

		// Keyboard and mouse mode: profile menu and the mapping editor.
		if (gKeyboardMode) {
			NSBox *separator = [[NSBox alloc] init];
			separator.boxType = NSBoxSeparator;
			separator.translatesAutoresizingMaskIntoConstraints = NO;
			[separator.widthAnchor constraintEqualToConstant:width].active = YES;

			NSTextField *profileLabel = PPText(@"Profile:", [NSFont systemFontOfSize:13], [NSColor labelColor]);
			gMainProfilePopup = PPMakeProfilePopup();
			gMainProfilePopup.target = gDelegate;
			gMainProfilePopup.action = @selector(profileChanged:);
			NSButton *edit = [NSButton buttonWithTitle:@"Edit Mappings…" target:gDelegate action:@selector(showMappings:)];
			NSStackView *profileRow = [NSStackView stackViewWithViews:@[profileLabel, gMainProfilePopup, edit]];
			profileRow.orientation = NSUserInterfaceLayoutOrientationHorizontal;
			profileRow.alignment = NSLayoutAttributeFirstBaseline;
			profileRow.spacing = 8;
			[views addObjectsFromArray:@[separator, profileRow]];
		}

		NSStackView *stack = [NSStackView stackViewWithViews:views];
		stack.orientation = NSUserInterfaceLayoutOrientationVertical;
		stack.alignment = NSLayoutAttributeCenterX;
		stack.spacing = 12;
		stack.edgeInsets = NSEdgeInsetsMake(20, 30, 24, 30);
		stack.translatesAutoresizingMaskIntoConstraints = NO;
		[stack setCustomSpacing:4 afterView:headline];
		[stack setCustomSpacing:18 afterView:gStatusDetail];
		[stack setCustomSpacing:18 afterView:gPermissionBanner];
		[stack setCustomSpacing:16 afterView:qrBox];
		[stack setCustomSpacing:6 afterView:codeRow];
		[stack setCustomSpacing:18 afterView:gStats];
		[stack setCustomSpacing:18 afterView:addrField];

		NSView *content = gWindow.contentView;
		[content addSubview:stack];
		[NSLayoutConstraint activateConstraints:@[
			[stack.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
			[stack.trailingAnchor constraintEqualToAnchor:content.trailingAnchor],
			[stack.topAnchor constraintEqualToAnchor:content.topAnchor],
			[stack.bottomAnchor constraintEqualToAnchor:content.bottomAnchor],
		]];
		[content layoutSubtreeIfNeeded];
		[gWindow setContentSize:stack.fittingSize];
		[gWindow center];

		if (gKeyboardMode) {
			PPShowProfile(PPCurrentProfile());
		}
		[gWindow makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];
		const char *snapshotDir = getenv("POCKETPAD_SNAPSHOT_DIR");
		if (snapshotDir != NULL && snapshotDir[0] != 0) {
			NSString *dir = [NSString stringWithUTF8String:snapshotDir];
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
				PPRunSnapshots(dir);
			});
		}
		[app run];
	}
}

// ---------------------------------------------------------------------------
// Mapping editor
// ---------------------------------------------------------------------------

/** Display names for mapping actions; values are the mapping file's names. */
static NSString *PPActionTitle(NSString *value) {
	static NSDictionary *names = nil;
	if (names == nil) {
		names = @{
			@"none": @"None", @"mouseLeft": @"Left Click", @"mouseRight": @"Right Click",
			@"wheelUp": @"Scroll Up", @"wheelDown": @"Scroll Down",
			@"space": @"Space", @"enter": @"Return", @"esc": @"Escape", @"tab": @"Tab",
			@"shift": @"Shift", @"ctrl": @"Control", @"alt": @"Option", @"cmd": @"Command",
			@"up": @"Up Arrow", @"down": @"Down Arrow", @"left": @"Left Arrow", @"right": @"Right Arrow",
			@"home": @"Home", @"end": @"End", @"pageup": @"Page Up", @"pagedown": @"Page Down",
			@"delete": @"Delete", @"forwarddelete": @"Forward Delete", @"help": @"Help", @"capslock": @"Caps Lock",
			@"minus": @"-", @"equal": @"=", @"comma": @",", @"period": @".", @"slash": @"/",
			@"backslash": @"\\", @"semicolon": @";", @"quote": @"'", @"grave": @"`",
			@"lbracket": @"[", @"rbracket": @"]",
		};
	}
	NSString *name = names[value];
	if (name != nil) {
		return name;
	}
	if (value.length == 1 || [value hasPrefix:@"f"]) {
		return value.uppercaseString; // letters, digits, F1-F20
	}
	return value;
}

static NSArray *PPActionValues(void) {
	static NSMutableArray *values = nil;
	if (values == nil) {
		values = [NSMutableArray arrayWithObjects:
			@"none", @"mouseLeft", @"mouseRight", @"wheelUp", @"wheelDown",
			@"space", @"enter", @"esc", @"tab", @"shift", @"ctrl", @"alt", @"cmd",
			@"up", @"down", @"left", @"right",
			@"home", @"end", @"pageup", @"pagedown",
			@"delete", @"forwarddelete", @"help", @"capslock",
			@"minus", @"equal", @"comma", @"period", @"slash", @"backslash",
			@"semicolon", @"quote", @"grave", @"lbracket", @"rbracket", nil];
		for (char c = 'a'; c <= 'z'; c++) {
			[values addObject:[NSString stringWithFormat:@"%c", c]];
		}
		for (char c = '0'; c <= '9'; c++) {
			[values addObject:[NSString stringWithFormat:@"%c", c]];
		}
		for (int i = 1; i <= 20; i++) {
			[values addObject:[NSString stringWithFormat:@"f%d", i]];
		}
	}
	return values;
}

static NSPopUpButton *PPMakeActionPopup(NSRect frame) {
	NSPopUpButton *popup = [[NSPopUpButton alloc] initWithFrame:frame pullsDown:NO];
	for (NSString *value in PPActionValues()) {
		PPAddItem(popup, PPActionTitle(value), value);
	}
	return popup;
}

static void PPEditorStatus(NSString *text) {
	if (gEditorStatus != nil) {
		gEditorStatus.stringValue = text ?: @"";
	}
}

/** The mapping rows, grouped into editor sections. */
static NSArray *PPSections(void) {
	return @[
		@{@"title": @"Buttons", @"rows": @[
			@[@"buttons.A", @"A"], @[@"buttons.B", @"B"], @[@"buttons.X", @"X"], @[@"buttons.Y", @"Y"],
			@[@"buttons.LB", @"LB"], @[@"buttons.RB", @"RB"],
			@[@"buttons.Back", @"Back"], @[@"buttons.Start", @"Start"], @[@"buttons.Guide", @"Guide"],
			@[@"buttons.LS", @"Left stick click"], @[@"buttons.RS", @"Right stick click"],
		]},
		@{@"title": @"D-pad", @"rows": @[
			@[@"dpad.up", @"Up"], @[@"dpad.down", @"Down"],
			@[@"dpad.left", @"Left"], @[@"dpad.right", @"Right"],
		]},
		@{@"title": @"Left stick", @"rows": @[
			@[@"leftStick.up", @"Up"], @[@"leftStick.down", @"Down"],
			@[@"leftStick.left", @"Left"], @[@"leftStick.right", @"Right"],
		]},
		@{@"title": @"Triggers", @"rows": @[
			@[@"leftTrigger", @"LT"], @[@"rightTrigger", @"RT"],
		]},
		@{@"title": @"Right stick", @"rows": @[
			@[@"rightStick.mode", @"Behavior"],
			@[@"rightStick.up", @"Up (keys mode)"], @[@"rightStick.down", @"Down (keys mode)"],
			@[@"rightStick.left", @"Left (keys mode)"], @[@"rightStick.right", @"Right (keys mode)"],
		]},
	];
}

void pp_app_show_editor(const char *json) {
	@autoreleasepool {
		NSString *text = [NSString stringWithUTF8String:(json != NULL ? json : "{}")];
		id parsed = [NSJSONSerialization JSONObjectWithData:[text dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL];
		gJSON = [parsed isKindOfClass:[NSDictionary class]] ? parsed : @{};
		if (json != NULL) {
			free((void *)json);
		}

		const CGFloat W = 520;
		const CGFloat H = 620;
		const CGFloat rowH = 30;
		const CGFloat headerH = 30;
		const CGFloat popupW = 180;

		if (gEditorWindow == nil) {
			gEditorWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, W, H)
			                                            styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
			                                              backing:NSBackingStoreBuffered
			                                                defer:NO];
			gEditorWindow.title = @"Mappings";
			gEditorWindow.subtitle = @"Keyboard and mouse";
			gEditorWindow.releasedWhenClosed = NO;
			gEditorWindow.minSize = NSMakeSize(480, 420);
			[gEditorWindow center];
			NSView *content = gEditorWindow.contentView;

			// Top: profile menu and explanation.
			NSTextField *profileLabel = PPText(@"Profile:", [NSFont systemFontOfSize:13], [NSColor labelColor]);
			gEditorProfilePopup = PPMakeProfilePopup();
			gEditorProfilePopup.target = gDelegate;
			gEditorProfilePopup.action = @selector(profileChanged:);
			gEditorProfilePopup.translatesAutoresizingMaskIntoConstraints = NO;
			NSTextField *hint = [NSTextField wrappingLabelWithString:@"Choose a key for each control. Save stores the result as the Custom profile and applies it immediately."];
			hint.font = [NSFont systemFontOfSize:12];
			hint.textColor = [NSColor secondaryLabelColor];
			hint.translatesAutoresizingMaskIntoConstraints = NO;
			[content addSubview:profileLabel];
			[content addSubview:gEditorProfilePopup];
			[content addSubview:hint];

			// Middle: scrolling list of grouped rows.
			NSScrollView *scroll = [[NSScrollView alloc] init];
			scroll.hasVerticalScroller = YES;
			scroll.borderType = NSNoBorder;
			scroll.drawsBackground = NO;
			scroll.translatesAutoresizingMaskIntoConstraints = NO;
			[content addSubview:scroll];

			NSArray *sections = PPSections();
			CGFloat docWidth = W - 40;
			CGFloat docHeight = 0;
			for (NSDictionary *section in sections) {
				docHeight += headerH + [(NSArray *)section[@"rows"] count] * rowH + 12 + 16;
			}
			PPFlippedView *doc = [[PPFlippedView alloc] initWithFrame:NSMakeRect(0, 0, docWidth, docHeight)];
			doc.autoresizingMask = NSViewWidthSizable;
			scroll.documentView = doc;

			gRowKeys = [NSMutableArray array];
			gRowPopups = [NSMutableArray array];
			CGFloat top = 0;
			for (NSDictionary *section in sections) {
				NSArray *rows = section[@"rows"];
				NSTextField *title = PPText(section[@"title"], [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold], [NSColor labelColor]);
				title.translatesAutoresizingMaskIntoConstraints = YES;
				title.frame = NSMakeRect(4, top + 6, 300, 18);
				[doc addSubview:title];

				CGFloat groupH = rows.count * rowH + 12;
				NSBox *group = PPGroup();
				group.frame = NSMakeRect(0, top + headerH, docWidth, groupH);
				group.autoresizingMask = NSViewWidthSizable;
				[doc addSubview:group];

				// NSBox's content view uses bottom-up coordinates.
				NSUInteger i = 0;
				for (NSArray *row in rows) {
					CGFloat y = groupH - 6 - (i + 1) * rowH;
					NSTextField *label = PPText(row[1], [NSFont systemFontOfSize:13], [NSColor labelColor]);
					label.translatesAutoresizingMaskIntoConstraints = YES;
					label.frame = NSMakeRect(14, y + 6, 220, 18);
					[group.contentView addSubview:label];

					NSRect popupFrame = NSMakeRect(docWidth - 14 - popupW, y + 2, popupW, 26);
					NSPopUpButton *popup;
					NSString *key = row[0];
					if ([key isEqualToString:@"rightStick.mode"]) {
						popup = [[NSPopUpButton alloc] initWithFrame:popupFrame pullsDown:NO];
						PPAddItem(popup, @"Mouse look", @"mouse");
						PPAddItem(popup, @"Keys", @"keys");
					} else {
						popup = PPMakeActionPopup(popupFrame);
					}
					popup.autoresizingMask = NSViewMinXMargin;
					[group.contentView addSubview:popup];
					[gRowKeys addObject:key];
					[gRowPopups addObject:popup];
					i++;
				}
				top += headerH + groupH + 16;
			}

			// Bottom: status and buttons (Revert on the left, Save as default).
			gEditorStatus = PPText(@"", [NSFont systemFontOfSize:12], [NSColor secondaryLabelColor]);
			gEditorStatus.lineBreakMode = NSLineBreakByTruncatingTail;
			[gEditorStatus setContentCompressionResistancePriority:NSLayoutPriorityDefaultLow forOrientation:NSLayoutConstraintOrientationHorizontal];
			NSButton *revert = [NSButton buttonWithTitle:@"Revert" target:gDelegate action:@selector(revertMapping:)];
			NSButton *close = [NSButton buttonWithTitle:@"Close" target:gDelegate action:@selector(closeEditor:)];
			close.keyEquivalent = @"\e";
			NSButton *save = [NSButton buttonWithTitle:@"Save" target:gDelegate action:@selector(saveMapping:)];
			save.keyEquivalent = @"\r";
			for (NSView *v in @[revert, close, save]) {
				v.translatesAutoresizingMaskIntoConstraints = NO;
				[content addSubview:v];
			}
			[content addSubview:gEditorStatus];

			[NSLayoutConstraint activateConstraints:@[
				[profileLabel.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
				[gEditorProfilePopup.leadingAnchor constraintEqualToAnchor:profileLabel.trailingAnchor constant:8],
				[gEditorProfilePopup.topAnchor constraintEqualToAnchor:content.topAnchor constant:16],
				[profileLabel.firstBaselineAnchor constraintEqualToAnchor:gEditorProfilePopup.firstBaselineAnchor],
				[hint.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
				[hint.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
				[hint.topAnchor constraintEqualToAnchor:gEditorProfilePopup.bottomAnchor constant:10],

				[scroll.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
				[scroll.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
				[scroll.topAnchor constraintEqualToAnchor:hint.bottomAnchor constant:14],
				[scroll.bottomAnchor constraintEqualToAnchor:save.topAnchor constant:-16],

				[save.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
				[save.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-16],
				[close.trailingAnchor constraintEqualToAnchor:save.leadingAnchor constant:-8],
				[close.firstBaselineAnchor constraintEqualToAnchor:save.firstBaselineAnchor],
				[revert.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
				[revert.firstBaselineAnchor constraintEqualToAnchor:save.firstBaselineAnchor],
				[gEditorStatus.leadingAnchor constraintEqualToAnchor:revert.trailingAnchor constant:12],
				[gEditorStatus.trailingAnchor constraintLessThanOrEqualToAnchor:close.leadingAnchor constant:-12],
				[gEditorStatus.firstBaselineAnchor constraintEqualToAnchor:save.firstBaselineAnchor],
			]];
		}

		PPFillRowsFromJSON();
		[gEditorWindow makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
	}
}

static void PPFillRowsFromJSON(void) {
	NSDictionary *buttons = gJSON[@"buttons"];
	NSDictionary *dpad = gJSON[@"dpad"];
	NSDictionary *leftStick = gJSON[@"leftStick"];
	id rightStick = gJSON[@"rightStick"];

	for (NSUInteger i = 0; i < gRowKeys.count; i++) {
		NSString *key = gRowKeys[i];
		NSPopUpButton *popup = gRowPopups[i];

		if ([key isEqualToString:@"rightStick.mode"]) {
			// "mouse" (or absent) is mouse look; an object of keys, or "none"
			// in a hand-written file, is keys mode.
			BOOL keys = [rightStick isKindOfClass:[NSDictionary class]] ||
			            ([rightStick isKindOfClass:[NSString class]] && [(NSString *)rightStick caseInsensitiveCompare:@"none"] == NSOrderedSame);
			PPSelectValue(popup, keys ? @"keys" : @"mouse");
			continue;
		}

		id value = nil;
		if ([key isEqualToString:@"leftTrigger"] || [key isEqualToString:@"rightTrigger"]) {
			value = gJSON[key];
		} else {
			NSArray *parts = [key componentsSeparatedByString:@"."];
			NSString *group = parts[0];
			NSString *name = parts[1];
			if ([group isEqualToString:@"buttons"] && [buttons isKindOfClass:[NSDictionary class]]) {
				value = buttons[name];
			} else if ([group isEqualToString:@"dpad"] && [dpad isKindOfClass:[NSDictionary class]]) {
				value = dpad[name];
			} else if ([group isEqualToString:@"leftStick"] && [leftStick isKindOfClass:[NSDictionary class]]) {
				value = leftStick[name];
			} else if ([group isEqualToString:@"rightStick"] && [rightStick isKindOfClass:[NSDictionary class]]) {
				value = rightStick[name];
			}
		}
		NSString *v = [value isKindOfClass:[NSString class]] ? value : @"none";
		if (!PPSelectValue(popup, v)) {
			// Keep names from a hand-edited file (kp3, f13...) selectable.
			PPAddItem(popup, PPActionTitle(v), v);
			PPSelectValue(popup, v);
		}
	}
	PPShowProfile(PPCurrentProfile());
	PPEditorStatus(nil);
}

static NSDictionary *PPBuildDoc(void) {
	NSMutableDictionary *buttons = [NSMutableDictionary dictionary];
	NSMutableDictionary *dpad = [NSMutableDictionary dictionary];
	NSMutableDictionary *leftStick = [NSMutableDictionary dictionary];
	NSMutableDictionary *rightStick = [NSMutableDictionary dictionary];
	BOOL rightStickKeys = NO;
	NSString *leftTrigger = @"none";
	NSString *rightTrigger = @"none";

	for (NSUInteger i = 0; i < gRowKeys.count; i++) {
		NSString *key = gRowKeys[i];
		NSString *value = PPSelectedValue(gRowPopups[i]);
		if ([key isEqualToString:@"rightStick.mode"]) {
			rightStickKeys = [value isEqualToString:@"keys"];
			continue;
		}
		if ([key isEqualToString:@"leftTrigger"]) {
			leftTrigger = value;
			continue;
		}
		if ([key isEqualToString:@"rightTrigger"]) {
			rightTrigger = value;
			continue;
		}
		NSArray *parts = [key componentsSeparatedByString:@"."];
		NSString *group = parts[0];
		NSString *name = parts[1];
		if ([group isEqualToString:@"buttons"]) {
			if (![value isEqualToString:@"none"]) {
				buttons[name] = value;
			}
		} else if ([group isEqualToString:@"dpad"]) {
			dpad[name] = value;
		} else if ([group isEqualToString:@"leftStick"]) {
			leftStick[name] = value;
		} else if ([group isEqualToString:@"rightStick"]) {
			rightStick[name] = value;
		}
	}

	return @{
		@"name": @"custom",
		@"buttons": buttons,
		@"dpad": dpad,
		@"leftStick": leftStick,
		@"rightStick": rightStickKeys ? rightStick : @"mouse",
		@"leftTrigger": leftTrigger,
		@"rightTrigger": rightTrigger,
	};
}
