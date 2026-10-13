// The native macOS window for `pocketpad app`: a QR code the phone scans,
// the pairing code, live connection status and a GUI mapping editor.
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

// --- palette ---------------------------------------------------------------

/** Top-down coordinate content view for scroll documents. */
@interface PPFlippedView : NSView
@end

@implementation PPFlippedView
- (BOOL)isFlipped {
	return YES;
}
@end

static NSColor *PPBackground(void) { return [NSColor colorWithSRGBRed:0.043 green:0.051 blue:0.071 alpha:1.0]; }
static NSColor *PPElevated(void) { return [NSColor colorWithSRGBRed:0.075 green:0.086 blue:0.118 alpha:1.0]; }
static NSColor *PPChip(void) { return [NSColor colorWithSRGBRed:0.11 green:0.125 blue:0.17 alpha:1.0]; }
static NSColor *PPAccent(void) { return [NSColor colorWithSRGBRed:0.486 green:0.361 blue:1.0 alpha:1.0]; }

/** A rounded panel. */
static NSView *PPCard(NSRect frame, NSColor *color, CGFloat radius) {
	NSView *view = [[NSView alloc] initWithFrame:frame];
	view.wantsLayer = YES;
	view.layer.backgroundColor = color.CGColor;
	view.layer.cornerRadius = radius;
	return view;
}

/** Small uppercase caption. */
static NSTextField *PPCaption(NSRect frame, NSString *text) {
	NSTextField *label = [NSTextField labelWithString:text];
	label.font = [NSFont systemFontOfSize:10 weight:NSFontWeightSemibold];
	label.textColor = [NSColor tertiaryLabelColor];
	return label;
}

static NSTextField *PPLabel(NSRect frame, NSString *text, CGFloat size, NSColor *color) {
	NSTextField *label = [NSTextField labelWithString:text];
	label.font = [NSFont systemFontOfSize:size];
	label.textColor = color;
	return label;
}

// --- main window state -----------------------------------------------------

static NSWindow *gWindow = nil;
static NSTextField *gStatus = nil;
static NSTextField *gPillText = nil;
static NSString *gCodeString = nil;

// Editor state.
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
- (void)copyCode:(id)sender;
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
	PPEditorStatus(nil);
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

// Updates the status line and the header pill from any thread.
void pp_app_set_status(const char *text) {
	NSString *value = [NSString stringWithUTF8String:(text ? text : "")];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gStatus != nil) {
			gStatus.stringValue = value;
		}
		if (gPillText != nil) {
			NSColor *color = [NSColor systemOrangeColor];
			NSString *summary = @"Waiting";
			if ([value hasPrefix:@"●"]) {
				color = [NSColor systemGreenColor];
				summary = @"Connected";
			} else if ([value hasPrefix:@"⚠"]) {
				color = [NSColor systemOrangeColor];
				summary = @"Permission needed";
			}
			NSMutableAttributedString *line = [[NSMutableAttributedString alloc] initWithString:[NSString stringWithFormat:@"● %@", summary]];
			[line addAttribute:NSForegroundColorAttributeName value:color range:NSMakeRange(0, 1)];
			[line addAttribute:NSForegroundColorAttributeName value:[NSColor labelColor] range:NSMakeRange(1, line.length - 1)];
			[line addAttribute:NSFontAttributeName value:[NSFont systemFontOfSize:12] range:NSMakeRange(0, line.length)];
			gPillText.attributedStringValue = line;
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
		gCodeString = [NSString stringWithUTF8String:code];

		CGFloat W = 540;
		CGFloat H = 740;
		CGFloat (^topY)(CGFloat, CGFloat) = ^CGFloat(CGFloat top, CGFloat h) { return H - top - h; };

		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable;
		gWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, W, H) styleMask:style backing:NSBackingStoreBuffered defer:NO];
		gWindow.title = [NSString stringWithUTF8String:title];
		gWindow.releasedWhenClosed = NO;
		gWindow.backgroundColor = PPBackground();
		gWindow.titlebarAppearsTransparent = YES;
		gWindow.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
		[gWindow center];
		NSView *content = gWindow.contentView;

		// Header: logo, wordmark, live status pill.
		if (logo != NULL && logoLen > 0) {
			NSImageView *logoView = [[NSImageView alloc] initWithFrame:NSMakeRect(28, topY(26, 44), 44, 44)];
			logoView.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:logo length:logoLen]];
			logoView.imageScaling = NSImageScaleProportionallyUpOrDown;
			logoView.wantsLayer = YES;
			logoView.layer.cornerRadius = 10;
			logoView.layer.masksToBounds = YES;
			[content addSubview:logoView];
		}
		NSTextField *wordmark = [NSTextField labelWithString:@"PocketPad"];
		wordmark.font = [NSFont boldSystemFontOfSize:21];
		wordmark.frame = NSMakeRect(86, topY(37, 26), 220, 26);
		[content addSubview:wordmark];

		NSView *pill = PPCard(NSMakeRect(W - 28 - 240, topY(30, 32), 240, 32), PPChip(), 16);
		[content addSubview:pill];
		gPillText = PPLabel(NSMakeRect(0, 8, 240, 16), @"◌ Starting…", 12, [NSColor labelColor]);
		gPillText.alignment = NSTextAlignmentCenter;
		[pill addSubview:gPillText];

		NSView *divider = [[NSView alloc] initWithFrame:NSMakeRect(0, topY(90, 1), W, 1)];
		divider.wantsLayer = YES;
		divider.layer.backgroundColor = [[NSColor colorWithWhite:1 alpha:0.07] CGColor];
		[content addSubview:divider];

		// Pairing card: QR with a glow, caption, code chip with Copy, address.
		NSView *hero = PPCard(NSMakeRect(28, topY(112, 400), W - 56, 400), PPElevated(), 16);
		[content addSubview:hero];

		CGFloat heroW = W - 56;
		NSView *qrCard = [[NSView alloc] initWithFrame:NSMakeRect((heroW - 250) / 2, 400 - 24 - 250, 250, 250)];
		qrCard.wantsLayer = YES;
		qrCard.layer.backgroundColor = [[NSColor whiteColor] CGColor];
		qrCard.layer.cornerRadius = 14;
		qrCard.layer.shadowColor = PPAccent().CGColor;
		qrCard.layer.shadowOpacity = 0.5;
		qrCard.layer.shadowRadius = 18;
		qrCard.layer.shadowOffset = CGSizeMake(0, -2);
		[hero addSubview:qrCard];

		NSImageView *qr = [[NSImageView alloc] initWithFrame:NSMakeRect(13, 13, 224, 224)];
		qr.image = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:pngLen]];
		qr.imageScaling = NSImageScaleProportionallyUpOrDown;
		[qrCard addSubview:qr];

		NSTextField *scanHelp = PPCaption(NSZeroRect, @"SCAN WITH THE POCKETPAD APP, OR TYPE THE CODE");
		scanHelp.alignment = NSTextAlignmentCenter;
		scanHelp.frame = NSMakeRect(0, 400 - 292 - 14, heroW, 14);
		[hero addSubview:scanHelp];

		NSView *codeChip = PPCard(NSMakeRect(42, 400 - 316 - 44, heroW - 84, 44), PPChip(), 10);
		[hero addSubview:codeChip];
		NSTextField *codeField = [NSTextField labelWithString:gCodeString];
		codeField.font = [NSFont monospacedSystemFontOfSize:15 weight:NSFontWeightSemibold];
		codeField.textColor = PPAccent();
		codeField.alignment = NSTextAlignmentCenter;
		codeField.selectable = YES;
		codeField.frame = NSMakeRect(12, 13, codeChip.frame.size.width - 108, 18);
		[codeChip addSubview:codeField];
		NSButton *copy = [NSButton buttonWithTitle:@"Copy" target:gDelegate action:@selector(copyCode:)];
		copy.bezelStyle = NSBezelStyleRounded;
		copy.controlSize = NSControlSizeSmall;
		copy.frame = NSMakeRect(codeChip.frame.size.width - 88, 8, 78, 28);
		[codeChip addSubview:copy];

		NSTextField *addrField = [NSTextField labelWithString:[NSString stringWithUTF8String:address]];
		addrField.font = [NSFont monospacedSystemFontOfSize:11 weight:NSFontWeightRegular];
		addrField.textColor = [NSColor secondaryLabelColor];
		addrField.alignment = NSTextAlignmentCenter;
		addrField.selectable = YES;
		addrField.frame = NSMakeRect(0, 16, heroW, 16);
		[hero addSubview:addrField];

		// Live status card.
		NSView *statusCard = PPCard(NSMakeRect(28, topY(528, 100), W - 56, 100), PPElevated(), 16);
		[content addSubview:statusCard];
		NSTextField *statusCap = PPCaption(NSZeroRect, @"LIVE STATUS");
		statusCap.frame = NSMakeRect(18, 100 - 14 - 16, 200, 14);
		[statusCard addSubview:statusCap];
		gStatus = [NSTextField labelWithString:@"Starting…"];
		gStatus.font = [NSFont systemFontOfSize:11];
		gStatus.textColor = [NSColor secondaryLabelColor];
		gStatus.maximumNumberOfLines = 3;
		gStatus.lineBreakMode = NSLineBreakByWordWrapping;
		gStatus.frame = NSMakeRect(18, 12, W - 56 - 36, 56);
		[statusCard addSubview:gStatus];

		// Bottom bar.
		NSButton *mappings = [NSButton buttonWithTitle:@"Mapping editor…" target:gDelegate action:@selector(showMappings:)];
		mappings.bezelStyle = NSBezelStyleRounded;
		mappings.bezelColor = PPAccent();
		mappings.contentTintColor = [NSColor whiteColor];
		mappings.frame = NSMakeRect((W - 240) / 2, topY(652, 40), 240, 40);
		[content addSubview:mappings];

		[gWindow makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];
		[app run];
	}
}

// ---------------------------------------------------------------------------
// Mapping editor: dropdowns grouped in sections, no file editing required.
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
	NSPopUpButton *popup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(0, 0, 190, 26) pullsDown:NO];
	[popup addItemsWithTitles:PPActionTitles()];
	if (value.length > 0 && [popup itemWithTitle:value] == nil) {
		// Keep exotic names (kp3, f13...) that came from a hand-edited file.
		[popup addItemWithTitle:value];
	}
	[popup selectItemWithTitle:(value.length > 0 ? value : @"none")];
	return popup;
}

static void PPEditorStatus(NSString *text) {
	gEditorStatus.stringValue = text ? text : @"";
}

/** The mapping rows, grouped into editor sections. */
static NSArray *PPSections(void) {
	return @[
		@{@"title": @"BUTTONS", @"rows": @[
			@[@"buttons.A", @"A"], @[@"buttons.B", @"B"], @[@"buttons.X", @"X"], @[@"buttons.Y", @"Y"],
			@[@"buttons.LB", @"LB"], @[@"buttons.RB", @"RB"],
			@[@"buttons.Back", @"Back"], @[@"buttons.Start", @"Start"], @[@"buttons.Guide", @"Guide"],
			@[@"buttons.LS", @"LS click"], @[@"buttons.RS", @"RS click"],
		]},
		@{@"title": @"D-PAD", @"rows": @[
			@[@"dpad.up", @"Up"], @[@"dpad.down", @"Down"],
			@[@"dpad.left", @"Left"], @[@"dpad.right", @"Right"],
		]},
		@{@"title": @"LEFT STICK", @"rows": @[
			@[@"leftStick.up", @"Up"], @[@"leftStick.down", @"Down"],
			@[@"leftStick.left", @"Left"], @[@"leftStick.right", @"Right"],
		]},
		@{@"title": @"TRIGGERS", @"rows": @[
			@[@"leftTrigger", @"LT"], @[@"rightTrigger", @"RT"],
		]},
		@{@"title": @"RIGHT STICK", @"rows": @[
			@[@"rightStick.mode", @"Behaviour"],
			@[@"rightStick.up", @"Up (keys mode)"], @[@"rightStick.down", @"Down (keys mode)"],
			@[@"rightStick.left", @"Left (keys mode)"], @[@"rightStick.right", @"Right (keys mode)"],
		]},
	];
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

		CGFloat W = 560;
		CGFloat H = 640;
		CGFloat (^topY)(CGFloat, CGFloat) = ^CGFloat(CGFloat top, CGFloat h) { return H - top - h; };

		if (gEditorWindow == nil) {
			NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable;
			gEditorWindow = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, W, H) styleMask:style backing:NSBackingStoreBuffered defer:NO];
			gEditorWindow.title = @"PocketPad — Mappings";
			gEditorWindow.releasedWhenClosed = NO;
			gEditorWindow.backgroundColor = PPBackground();
			gEditorWindow.titlebarAppearsTransparent = YES;
			gEditorWindow.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
			gEditorWindow.minSize = NSMakeSize(520, 480);
			[gEditorWindow center];
			NSView *content = gEditorWindow.contentView;

			NSTextField *heading = [NSTextField labelWithString:@"Mappings"];
			heading.font = [NSFont boldSystemFontOfSize:18];
			heading.frame = NSMakeRect(24, topY(24, 24), 200, 24);
			[content addSubview:heading];

			NSTextField *profileLabel = PPLabel(NSZeroRect, @"Profile", 12, [NSColor secondaryLabelColor]);
			profileLabel.frame = NSMakeRect(W - 24 - 190 - 60, topY(28, 18), 54, 18);
			[content addSubview:profileLabel];

			gProfilePopup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(W - 24 - 190, topY(24, 28), 190, 28) pullsDown:NO];
			[gProfilePopup addItemsWithTitles:@[@"general", @"racing", @"gta", @"custom"]];
			gProfilePopup.target = gDelegate;
			gProfilePopup.action = @selector(profileChanged:);
			[content addSubview:gProfilePopup];

			NSTextField *hint = PPLabel(NSZeroRect, @"The rows below edit the \"custom\" mapping; Save switches to it and applies it live.", 11, [NSColor secondaryLabelColor]);
			hint.frame = NSMakeRect(24, topY(56, 16), W - 48, 16);
			[content addSubview:hint];

			NSTextField *sectionHeading = PPCaption(NSZeroRect, @"CONTROLS");
			sectionHeading.frame = NSMakeRect(24, topY(84, 14), 200, 14);
			[content addSubview:sectionHeading];

			NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSMakeRect(18, 64, W - 36, H - 64 - 100)];
			scroll.hasVerticalScroller = YES;
			scroll.borderType = NSNoBorder;
			scroll.drawsBackground = NO;
			scroll.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

			CGFloat docWidth = W - 36 - 2;
			NSArray *sections = PPSections();
			CGFloat docHeight = 8;
			for (NSDictionary *section in sections) {
				docHeight += 34 + [(NSArray *)section[@"rows"] count] * 30 + 12;
			}

			NSView *doc = [[PPFlippedView alloc] initWithFrame:NSMakeRect(0, 0, docWidth, docHeight)];
			[scroll setDocumentView:doc];

			gRowKeys = [NSMutableArray array];
			gRowPopups = [NSMutableArray array];

			CGFloat top = 8;
			for (NSDictionary *section in sections) {
				NSArray *rows = section[@"rows"];
				CGFloat cardHeight = 34 + rows.count * 30 + 6;
				NSView *card = PPCard(NSMakeRect(0, top, docWidth, cardHeight), PPElevated(), 12);
				[doc addSubview:card];

				NSTextField *title = PPCaption(NSZeroRect, section[@"title"]);
				title.frame = NSMakeRect(16, cardHeight - 14 - 12, 200, 14);
				[card addSubview:title];

				NSUInteger i = 0;
				for (NSArray *row in rows) {
					NSString *key = row[0];
					NSTextField *label = PPLabel(NSZeroRect, row[1], 12, [NSColor labelColor]);
					label.frame = NSMakeRect(16, cardHeight - 34 - (i + 1) * 30 + 6, 200, 18);
					[card addSubview:label];

					NSPopUpButton *popup;
					if ([key isEqualToString:@"rightStick.mode"]) {
						popup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(docWidth - 16 - 190, label.frame.origin.y - 4, 190, 26) pullsDown:NO];
						[popup addItemsWithTitles:@[@"Mouse look", @"Keys"]];
					} else {
						popup = PPMakeActionPopup(nil);
						[popup setFrame:NSMakeRect(docWidth - 16 - 190, label.frame.origin.y - 4, 190, 26)];
					}
					[card addSubview:popup];
					[gRowKeys addObject:key];
					[gRowPopups addObject:popup];
					i++;
				}
				top += cardHeight + 12;
			}

			[content addSubview:scroll];

			gEditorStatus = PPLabel(NSZeroRect, @"", 11, [NSColor secondaryLabelColor]);
			gEditorStatus.frame = NSMakeRect(24, topY(H - 36, 16), W - 48 - 330, 16);
			[content addSubview:gEditorStatus];

			CGFloat buttonY = topY(H - 44, 32);
			NSButton *closeButton = [NSButton buttonWithTitle:@"Close" target:gDelegate action:@selector(closeEditor:)];
			closeButton.frame = NSMakeRect(W - 24 - 90, buttonY, 90, 32);
			closeButton.autoresizingMask = NSViewMinXMargin;
			[content addSubview:closeButton];

			NSButton *reload = [NSButton buttonWithTitle:@"Revert" target:gDelegate action:@selector(reloadMapping:)];
			reload.frame = NSMakeRect(W - 24 - 90 - 8 - 90, buttonY, 90, 32);
			reload.autoresizingMask = NSViewMinXMargin;
			[content addSubview:reload];

			NSButton *save = [NSButton buttonWithTitle:@"Save & use" target:gDelegate action:@selector(saveMapping:)];
			save.bezelStyle = NSBezelStyleRounded;
			save.bezelColor = PPAccent();
			save.contentTintColor = [NSColor whiteColor];
			save.frame = NSMakeRect(W - 24 - 90 - 8 - 90 - 8 - 116, buttonY, 116, 32);
			save.autoresizingMask = NSViewMinXMargin;
			[save setKeyEquivalent:@"\r"];
			[content addSubview:save];
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
	PPEditorStatus(nil);
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
