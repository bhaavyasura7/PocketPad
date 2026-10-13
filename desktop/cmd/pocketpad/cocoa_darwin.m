// The native macOS window for `pocketpad app`: a QR code the phone scans,
// the pairing code, the LAN address, live connection status and a GUI
// mapping editor. Compiled by cgo as Objective-C; Go only calls plain C
// entry points.

#import <Cocoa/Cocoa.h>
#include <stdint.h>
#include <stdlib.h>

// Implemented in Go (cgo //export in app_darwin.go). Returned strings are
// malloc'd and must be freed by the caller.
extern char *ppGoCurrentMappingJSON(void);
extern char *ppGoSaveMapping(const char *json);
extern char *ppGoSetProfile(const char *name);
extern char *ppGoCurrentProfileName(void);

static NSWindow *gWindow = nil;
static NSTextField *gStatus = nil;

// Forward declarations for the editor (defined below).
static NSWindow *gEditorWindow = nil;
static NSPopUpButton *gProfilePopup = nil;
static NSTextField *gEditorStatus = nil;
static NSMutableArray *gRowKeys = nil;
static NSMutableArray *gRowPopups = nil;
static NSDictionary *gJSON = nil;
void pp_app_show_editor(const char *json);
static void PPFillRowsFromJSON(void);
static NSDictionary *PPBuildDoc(void);
static void PPEditorStatus(NSString *text);

@interface PPAppDelegate : NSObject <NSApplicationDelegate>
- (void)showMappings:(id)sender;
- (void)profileChanged:(id)sender;
- (void)saveMapping:(id)sender;
- (void)reloadMapping:(id)sender;
- (void)closeEditor:(id)sender;
@end

@implementation PPAppDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
	return YES;
}

- (void)showMappings:(id)sender {
	pp_app_show_editor(ppGoCurrentMappingJSON());
}

- (void)profileChanged:(id)sender {
	NSPopUpButton *popup = (NSPopUpButton *)sender;
	NSString *name = popup.titleOfSelectedItem;
	char *err = ppGoSetProfile(name.UTF8String);
	if (err != NULL) {
		free((void *)err);
		if ([name isEqualToString:@"custom"]) {
			// No custom mapping exists yet: point at the rows instead of an
			// error, and let the user build one with Save.
			PPEditorStatus(@"No custom mapping yet — adjust the rows below and press Save & use.");
			return;
		}
		PPEditorStatus([NSString stringWithFormat:@"Could not switch to %@", name]);
	}
	char *current = ppGoCurrentMappingJSON();
	pp_app_show_editor(current); // refill rows from the now-active mapping
}

- (void)saveMapping:(id)sender {
	NSDictionary *doc = PPBuildDoc();
	NSError *encodeError = nil;
	NSData *data = [NSJSONSerialization dataWithJSONObject:doc options:NSJSONWritingPrettyPrinted error:&encodeError];
	if (data == nil) {
		gEditorStatus.stringValue = @"Could not encode the mapping.";
		return;
	}
	NSString *text = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
	char *err = ppGoSaveMapping(text.UTF8String);
	if (err != NULL) {
		NSString *message = [NSString stringWithUTF8String:err];
		free((void *)err);
		gEditorStatus.stringValue = @"Not saved — see the dialog.";
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = @"Mapping not saved";
		alert.informativeText = message;
		[alert runModal];
		return;
	}
	gEditorStatus.stringValue = @"Saved — active now";
	[gProfilePopup selectItemWithTitle:@"custom"];
}

- (void)reloadMapping:(id)sender {
	char *current = ppGoCurrentMappingJSON();
	pp_app_show_editor(current);
	gEditorStatus.stringValue = @"Reloaded from the active mapping";
}

- (void)closeEditor:(id)sender {
	[gEditorWindow orderOut:nil];
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
		app.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		gDelegate = [[PPAppDelegate alloc] init];
		[app setDelegate:gDelegate];

		NSColor *bg = [NSColor colorWithSRGBRed:0.043 green:0.051 blue:0.071 alpha:1.0];
		NSColor *accent = [NSColor colorWithSRGBRed:0.486 green:0.361 blue:1.0 alpha:1.0];
		NSColor *chip = [NSColor colorWithSRGBRed:0.094 green:0.106 blue:0.145 alpha:1.0];

		CGFloat W = 460;
		CGFloat H = 700;
		NSRect frame = NSMakeRect(0, 0, W, H);
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable;
		gWindow = [[NSWindow alloc] initWithContentRect:frame styleMask:style backing:NSBackingStoreBuffered defer:NO];
		gWindow.title = [NSString stringWithUTF8String:title];
		gWindow.releasedWhenClosed = NO;
		gWindow.backgroundColor = bg;
		gWindow.titlebarAppearsTransparent = YES;
		gWindow.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		[gWindow center];
		NSView *content = gWindow.contentView;

		CGFloat y = H - 34;

		// Centered logo and wordmark.
		if (logo != NULL && logoLen > 0) {
			NSImageView *logoView = [[NSImageView alloc] initWithFrame:NSMakeRect((W - 68) / 2, y - 68, 68, 68)];
			logoView.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:logo length:logoLen]];
			logoView.imageScaling = NSImageScaleProportionallyUpOrDown;
			logoView.wantsLayer = YES;
			logoView.layer.cornerRadius = 15;
			logoView.layer.masksToBounds = YES;
			[content addSubview:logoView];
			y -= 80;
		}

		NSTextField *heading = [NSTextField labelWithString:@"PocketPad"];
		heading.font = [NSFont boldSystemFontOfSize:26];
		heading.alignment = NSTextAlignmentCenter;
		heading.frame = NSMakeRect(0, y, W, 34);
		[content addSubview:heading];
		y -= 26;

		NSTextField *sub = [NSTextField labelWithString:@"Scan this with the PocketPad app on your phone.\nIt fills everything in for you."];
		sub.font = [NSFont systemFontOfSize:12];
		sub.textColor = [NSColor secondaryLabelColor];
		sub.alignment = NSTextAlignmentCenter;
		sub.maximumNumberOfLines = 2;
		sub.lineBreakMode = NSLineBreakByWordWrapping;
		sub.frame = NSMakeRect(20, y - 20, W - 40, 34);
		[content addSubview:sub];
		y -= 52;

		// QR in a white rounded card with a purple glow.
		CGFloat qrSize = 280;
		NSView *qrCard = [[NSView alloc] initWithFrame:NSMakeRect((W - qrSize) / 2, y - qrSize, qrSize, qrSize)];
		qrCard.wantsLayer = YES;
		qrCard.layer.backgroundColor = [[NSColor whiteColor] CGColor];
		qrCard.layer.cornerRadius = 18;
		qrCard.layer.shadowColor = accent.CGColor;
		qrCard.layer.shadowOpacity = 0.55;
		qrCard.layer.shadowRadius = 22;
		qrCard.layer.shadowOffset = CGSizeMake(0, -3);
		[content addSubview:qrCard];

		NSImageView *qr = [[NSImageView alloc] initWithFrame:NSMakeRect(14, 14, qrSize - 28, qrSize - 28)];
		qr.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:pngLen]];
		qr.imageScaling = NSImageScaleProportionallyUpOrDown;
		[qrCard addSubview:qr];
		y -= qrSize + 24;

		NSTextField *codeHelp = [NSTextField labelWithString:@"OR TYPE THIS CODE IN THE APP"];
		codeHelp.font = [NSFont systemFontOfSize:10 weight:NSFontWeightSemibold];
		codeHelp.textColor = [NSColor tertiaryLabelColor];
		codeHelp.alignment = NSTextAlignmentCenter;
		codeHelp.frame = NSMakeRect(20, y, W - 40, 14);
		[content addSubview:codeHelp];
		y -= 32;

		// Pairing code in a rounded chip, in the brand colour.
		NSView *codeChip = [[NSView alloc] initWithFrame:NSMakeRect(58, y, W - 116, 36)];
		codeChip.wantsLayer = YES;
		codeChip.layer.backgroundColor = chip.CGColor;
		codeChip.layer.cornerRadius = 9;
		[content addSubview:codeChip];

		NSTextField *codeField = [NSTextField labelWithString:[NSString stringWithUTF8String:code]];
		codeField.font = [NSFont monospacedSystemFontOfSize:15 weight:NSFontWeightSemibold];
		codeField.textColor = accent;
		codeField.alignment = NSTextAlignmentCenter;
		codeField.selectable = YES;
		codeField.frame = NSMakeRect(0, 8, W - 116, 20);
		[codeChip addSubview:codeField];
		y -= 26;

		NSTextField *addrField = [NSTextField labelWithString:[NSString stringWithUTF8String:address]];
		addrField.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
		addrField.textColor = [NSColor secondaryLabelColor];
		addrField.alignment = NSTextAlignmentCenter;
		addrField.selectable = YES;
		addrField.frame = NSMakeRect(20, y, W - 40, 18);
		[content addSubview:addrField];
		y -= 34;

		gStatus = [NSTextField labelWithString:@"Starting…"];
		gStatus.font = [NSFont systemFontOfSize:12];
		gStatus.textColor = [NSColor secondaryLabelColor];
		gStatus.alignment = NSTextAlignmentCenter;
		gStatus.maximumNumberOfLines = 3;
		gStatus.lineBreakMode = NSLineBreakByWordWrapping;
		gStatus.frame = NSMakeRect(20, y - 40, W - 40, 58);
		[content addSubview:gStatus];

		NSButton *mappings = [NSButton buttonWithTitle:@"Mapping editor…" target:gDelegate action:@selector(showMappings:)];
		mappings.frame = NSMakeRect((W - 220) / 2, 26, 220, 34);
		[content addSubview:mappings];

		[gWindow makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];
		[app run];
	}
}

// ---------------------------------------------------------------------------
// Mapping editor: dropdowns for every control, no file editing required.
// ---------------------------------------------------------------------------

static NSArray *gActionTitles = nil;

static NSArray *PPActionTitles(void) {
	if (gActionTitles == nil) {
		NSMutableArray *t = [NSMutableArray arrayWithObjects:
			@"none", @"mouseLeft", @"mouseRight", @"wheelUp", @"wheelDown",
			@"space", @"enter", @"esc", @"tab", @"shift", @"ctrl", @"alt", @"cmd",
			@"up", @"down", @"left", @"right",
			@"home", @"end", @"pageup", @"pagedown",
			@"delete", @"forwarddelete", @"help", @"capslock",
			@"minus", @"equal", @"comma", @"period", @"slash", @"backslash",
			@"semicolon", @"quote", @"grave", @"lbracket", @"rbracket",
			nil];
		for (char c = 'a'; c <= 'z'; c++) {
			[t addObject:[NSString stringWithFormat:@"%c", c]];
		}
		for (char c = '0'; c <= '9'; c++) {
			[t addObject:[NSString stringWithFormat:@"%c", c]];
		}
		for (int i = 1; i <= 20; i++) {
			[t addObject:[NSString stringWithFormat:@"f%d", i]];
		}
		gActionTitles = t;
	}
	return gActionTitles;
}

static NSPopUpButton *PPMakeActionPopup(NSString *value) {
	NSPopUpButton *popup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(0, 0, 185, 26) pullsDown:NO];
	[popup addItemsWithTitles:PPActionTitles()];
	if (value.length > 0 && [popup itemWithTitle:value] == nil) {
		// Keep exotic names (kp3, f13...) that came from a hand-edited file.
		[popup addItemWithTitle:value];
	}
	[popup selectItemWithTitle:(value.length > 0 ? value : @"none")];
	return popup;
}

static void PPEditorStatus(NSString *text) {
	gEditorStatus.stringValue = text;
}

void pp_app_show_editor(const char *json) {
	@autoreleasepool {
		NSString *text = [NSString stringWithUTF8String:(json != NULL ? json : "{}")];
		NSData *data = [text dataUsingEncoding:NSUTF8StringEncoding];
		id parsed = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
		gJSON = [parsed isKindOfClass:[NSDictionary class]] ? parsed : @{};
		if (json != NULL) {
			free((void *)json);
		}

		if (gEditorWindow == nil) {
			NSColor *bg = [NSColor colorWithSRGBRed:0.043 green:0.051 blue:0.071 alpha:1.0];
			NSColor *elevated = [NSColor colorWithSRGBRed:0.071 green:0.082 blue:0.114 alpha:1.0];
			NSRect frame = NSMakeRect(0, 0, 400, 560);
			NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable;
			gEditorWindow = [[NSWindow alloc] initWithContentRect:frame styleMask:style backing:NSBackingStoreBuffered defer:NO];
			gEditorWindow.title = @"PocketPad — Mappings";
			gEditorWindow.releasedWhenClosed = NO;
			gEditorWindow.backgroundColor = bg;
			gEditorWindow.titlebarAppearsTransparent = YES;
			gEditorWindow.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
			[gEditorWindow center];
			NSView *content = gEditorWindow.contentView;

			NSTextField *head = [NSTextField labelWithString:@"Profile"];
			head.font = [NSFont systemFontOfSize:12 weight:NSFontWeightSemibold];
			head.frame = NSMakeRect(16, 520, 60, 20);
			[content addSubview:head];

			gProfilePopup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(80, 514, 170, 28) pullsDown:NO];
			[gProfilePopup addItemsWithTitles:@[@"general", @"racing", @"gta", @"custom"]];
			gProfilePopup.target = gDelegate;
			gProfilePopup.action = @selector(profileChanged:);
			[content addSubview:gProfilePopup];

			NSTextField *hint = [NSTextField labelWithString:@"The rows below edit the \"custom\" mapping; Save switches to it and applies it live."];
			hint.font = [NSFont systemFontOfSize:11];
			hint.textColor = [NSColor secondaryLabelColor];
			hint.frame = NSMakeRect(16, 494, 368, 16);
			[content addSubview:hint];

			NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSMakeRect(12, 60, 376, 426)];
			scroll.hasVerticalScroller = YES;
			scroll.borderType = NSNoBorder;
			scroll.drawsBackground = NO;
			NSView *doc = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 356, 10)];
			doc.wantsLayer = YES;
			doc.layer.backgroundColor = elevated.CGColor;
			doc.layer.cornerRadius = 10;
			[scroll setDocumentView:doc];
			[content addSubview:scroll];

			gRowKeys = [NSMutableArray array];
			gRowPopups = [NSMutableArray array];

			NSArray *rows = @[
				@[@"buttons.A", @"A"], @[@"buttons.B", @"B"], @[@"buttons.X", @"X"], @[@"buttons.Y", @"Y"],
				@[@"buttons.LB", @"LB"], @[@"buttons.RB", @"RB"],
				@[@"buttons.Back", @"Back"], @[@"buttons.Start", @"Start"], @[@"buttons.Guide", @"Guide"],
				@[@"buttons.LS", @"LS click"], @[@"buttons.RS", @"RS click"],
				@[@"dpad.up", @"D-pad up"], @[@"dpad.down", @"D-pad down"],
				@[@"dpad.left", @"D-pad left"], @[@"dpad.right", @"D-pad right"],
				@[@"leftStick.up", @"Left stick up"], @[@"leftStick.down", @"Left stick down"],
				@[@"leftStick.left", @"Left stick left"], @[@"leftStick.right", @"Left stick right"],
				@[@"leftTrigger", @"LT"], @[@"rightTrigger", @"RT"],
				@[@"rightStick.mode", @"Right stick"],
				@[@"rightStick.up", @"Right stick up"], @[@"rightStick.down", @"Right stick down"],
				@[@"rightStick.left", @"Right stick left"], @[@"rightStick.right", @"Right stick right"],
			];
			CGFloat rowHeight = 30;
			CGFloat docHeight = rows.count * rowHeight + 8;
			[doc setFrame:NSMakeRect(0, 0, 356, docHeight)];
			NSUInteger i = 0;
			for (NSArray *row in rows) {
				NSString *key = row[0];
				NSTextField *label = [NSTextField labelWithString:row[1]];
				label.font = [NSFont systemFontOfSize:12];
				label.frame = NSMakeRect(14, docHeight - (i + 1) * rowHeight + 6, 146, 18);
				[doc addSubview:label];

				NSPopUpButton *popup;
				if ([key isEqualToString:@"rightStick.mode"]) {
					popup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(164, docHeight - (i + 1) * rowHeight + 2, 182, 26) pullsDown:NO];
					[popup addItemsWithTitles:@[@"Mouse look", @"Keys"]];
				} else {
					popup = PPMakeActionPopup(nil);
					[popup setFrame:NSMakeRect(164, docHeight - (i + 1) * rowHeight + 2, 182, 26)];
				}
				[doc addSubview:popup];
				[gRowKeys addObject:key];
				[gRowPopups addObject:popup];
				i++;
			}

			NSTextField *status = [NSTextField labelWithString:@""];
			status.font = [NSFont systemFontOfSize:11];
			status.textColor = [NSColor secondaryLabelColor];
			status.frame = NSMakeRect(16, 40, 368, 16);
			[content addSubview:status];
			gEditorStatus = status;

			NSButton *save = [NSButton buttonWithTitle:@"Save & use" target:gDelegate action:@selector(saveMapping:)];
			save.frame = NSMakeRect(16, 8, 120, 30);
			[save setKeyEquivalent:@"\r"];
			[content addSubview:save];

			NSButton *reload = [NSButton buttonWithTitle:@"Revert" target:gDelegate action:@selector(reloadMapping:)];
			reload.frame = NSMakeRect(144, 8, 90, 30);
			[content addSubview:reload];

			NSButton *closeButton = [NSButton buttonWithTitle:@"Close" target:gDelegate action:@selector(closeEditor:)];
			closeButton.frame = NSMakeRect(300, 8, 84, 30);
			[content addSubview:closeButton];
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
	NSDictionary *rightStick = gJSON[@"rightStick"];

	for (NSUInteger i = 0; i < gRowKeys.count; i++) {
		NSString *key = gRowKeys[i];
		NSPopUpButton *popup = gRowPopups[i];

		if ([key isEqualToString:@"rightStick.mode"]) {
			BOOL mouse = YES;
			if ([rightStick isKindOfClass:[NSString class]]) {
				mouse = ![(NSString *)rightStick isEqualToString:@"none"];
			} else if ([rightStick isKindOfClass:[NSDictionary class]]) {
				mouse = NO;
			}
			[popup selectItemAtIndex:(mouse ? 0 : 1)];
			continue;
		}

		NSString *value = nil;
		if ([key isEqualToString:@"leftTrigger"]) {
			value = gJSON[@"leftTrigger"];
		} else if ([key isEqualToString:@"rightTrigger"]) {
			value = gJSON[@"rightTrigger"];
		} else {
			NSArray *parts = [key componentsSeparatedByString:@"."];
			NSString *group = parts[0];
			NSString *name = parts[1];
			if ([group isEqualToString:@"buttons"]) {
				value = buttons[name];
			} else if ([group isEqualToString:@"dpad"]) {
				value = dpad[name];
			} else if ([group isEqualToString:@"leftStick"]) {
				value = leftStick[name];
			} else if ([group isEqualToString:@"rightStick"] && [rightStick isKindOfClass:[NSDictionary class]]) {
				value = rightStick[name];
			}
		}
		NSString *v = [value isKindOfClass:[NSString class]] ? value : @"none";
		if (![v isEqualToString:@"none"] && [popup itemWithTitle:v] == nil) {
			[popup addItemWithTitle:v];
		}
		[popup selectItemWithTitle:v];
	}

	char *currentProfile = ppGoCurrentProfileName();
	NSString *profileName = [NSString stringWithUTF8String:(currentProfile != NULL ? currentProfile : "general")];
	if (currentProfile != NULL) {
		free((void *)currentProfile);
	}
	[gProfilePopup selectItemWithTitle:profileName];
	PPEditorStatus(@"");
}

static NSDictionary *PPBuildDoc(void) {
	NSMutableDictionary *buttons = [NSMutableDictionary dictionary];
	NSMutableDictionary *dpad = [NSMutableDictionary dictionary];
	NSMutableDictionary *leftStick = [NSMutableDictionary dictionary];
	NSMutableDictionary *rightStick = [NSMutableDictionary dictionary];
	BOOL rightStickKeys = NO;

	for (NSUInteger i = 0; i < gRowKeys.count; i++) {
		NSString *key = gRowKeys[i];
		NSPopUpButton *popup = gRowPopups[i];
		NSString *value = popup.titleOfSelectedItem ?: @"none";

		if ([key isEqualToString:@"rightStick.mode"]) {
			rightStickKeys = (popup.indexOfSelectedItem == 1);
			continue;
		}
		if ([key isEqualToString:@"leftTrigger"] || [key isEqualToString:@"rightTrigger"]) {
			continue; // handled below
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

	NSMutableDictionary *doc = [NSMutableDictionary dictionary];
	doc[@"name"] = @"custom";
	doc[@"buttons"] = buttons;
	doc[@"dpad"] = dpad;
	doc[@"leftStick"] = leftStick;
	doc[@"rightStick"] = rightStickKeys ? rightStick : @"mouse";
	doc[@"leftTrigger"] = ((NSPopUpButton *)gRowPopups[[gRowKeys indexOfObject:@"leftTrigger"]]).titleOfSelectedItem ?: @"none";
	doc[@"rightTrigger"] = ((NSPopUpButton *)gRowPopups[[gRowKeys indexOfObject:@"rightTrigger"]]).titleOfSelectedItem ?: @"none";
	return doc;
}
