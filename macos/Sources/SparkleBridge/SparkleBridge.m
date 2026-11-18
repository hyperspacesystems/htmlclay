#import <Cocoa/Cocoa.h>
#import "SparkleBridge.h"

// Sparkle's SUNoUpdateError. A check that finds nothing is not worth a log line.
static const NSInteger hcSUNoUpdateError = 1001;

// Sparkle is loaded at run time, not linked, so its headers are not used. These
// protocols declare only the methods called below, with Sparkle 2's signatures.
@protocol HCSPUUpdater <NSObject>
- (instancetype)initWithHostBundle:(NSBundle *)hostBundle applicationBundle:(NSBundle *)applicationBundle userDriver:(id)userDriver delegate:(id)delegate;
- (BOOL)startUpdater:(NSError **)error;
- (void)checkForUpdatesInBackground;
@property (nonatomic, readonly) BOOL canCheckForUpdates;
@property (nonatomic, readonly) BOOL sessionInProgress;
@end

@protocol HCSPUStandardUserDriver <NSObject>
- (instancetype)initWithHostBundle:(NSBundle *)hostBundle delegate:(id)delegate;
@end

@protocol HCSUAppcastItem <NSObject>
@property (nonatomic, copy, readonly) NSString *versionString;
@end

@class HCSparkleDelegate;

@interface HCSparkleBridge () {
@public
    void (^_events)(HCSparkleEvent event, NSString *text);
    NSString *_Nullable (^_skipReason)(NSString *version);
    id _updater;
    id _userDriver;
    void (^_installNow)(void);
    HCSparkleDelegate *_delegate;
}
@end

static void hcSend(HCSparkleBridge *bridge, HCSparkleEvent event, NSString *text) {
    void (^events)(HCSparkleEvent, NSString *) = bridge->_events;
    if (events != nil) {
        events(event, text ?: @"");
    }
}

// Every method runs on the main thread: Sparkle calls its delegate there, and
// -start, -installNow and -checkNow dispatch to it.
@interface HCSparkleDelegate : NSObject
@end

@implementation HCSparkleDelegate {
@public
    __weak HCSparkleBridge *owner;
}

// Sparkle calls this once it has downloaded, verified and prepared an update
// that it would otherwise install when the app quits. Returning YES keeps the
// block for the tray's Restart to Update row and stops further update cycles.
// Sparkle still installs on any quit, whatever this returns.
- (BOOL)updater:(id)u willInstallUpdateOnQuit:(id)item immediateInstallationBlock:(void (^)(void))block {
    HCSparkleBridge *bridge = owner;
    if (bridge == nil) {
        return YES;
    }
    bridge->_installNow = [block copy];
    hcSend(bridge, HCSparkleReady, [(id<HCSUAppcastItem>)item versionString]);
    return YES;
}

// Sparkle asks before it downloads an update. A version that failed to start
// after an earlier install is refused, so Sparkle never prepares it again and
// a plain quit cannot install it. A newer version is offered as usual.
- (BOOL)updater:(id)u shouldProceedWithUpdate:(id)item updateCheck:(NSInteger)check error:(NSError **)error {
    HCSparkleBridge *bridge = owner;
    if (bridge == nil) {
        return YES;
    }
    NSString *v = [(id<HCSUAppcastItem>)item versionString];
    NSString *_Nullable (^skipReason)(NSString *) = bridge->_skipReason;
    NSString *reason = skipReason != nil ? skipReason(v ?: @"") : nil;
    if (reason == nil || reason.length == 0) {
        return YES;
    }
    if (error != NULL) {
        *error = [NSError errorWithDomain:@"com.htmlclay.update" code:1 userInfo:@{NSLocalizedDescriptionKey: reason}];
    }
    return NO;
}

// Sparkle asks before any check, whether a person asked or its own timer did.
- (BOOL)updater:(id)u mayPerformUpdateCheck:(NSInteger)check error:(NSError **)error {
    HCSparkleBridge *bridge = owner;
    if (bridge != nil) {
        hcSend(bridge, HCSparkleChecking, @"");
    }
    return YES;
}

- (void)updater:(id)u didFinishUpdateCycleForUpdateCheck:(NSInteger)check error:(NSError *)error {
    HCSparkleBridge *bridge = owner;
    if (bridge == nil) {
        return;
    }
    hcSend(bridge, HCSparkleCheckEnded, @"");
}

- (void)updater:(id)u didAbortWithError:(NSError *)error {
    if ([error.domain isEqualToString:@"SUSparkleErrorDomain"] && error.code == hcSUNoUpdateError) {
        HCSparkleBridge *bridge = owner;
        if (bridge != nil) {
            hcSend(bridge, HCSparkleUpToDate, @"");
        }
        return;
    }
    HCSparkleBridge *bridge = owner;
    if (bridge == nil) {
        return;
    }
    hcSend(bridge, HCSparkleAborted, [NSString stringWithFormat:@"%@ (%@ %ld)", error.localizedDescription, error.domain, (long)error.code]);
}
@end

@implementation HCSparkleBridge

+ (nullable NSString *)loadFrameworkAtPath:(NSString *)path {
    NSBundle *framework = [NSBundle bundleWithPath:path];
    if (framework == nil) {
        return [NSString stringWithFormat:@"Sparkle.framework is not at %@", path];
    }
    NSError *error = nil;
    if (![framework loadAndReturnError:&error]) {
        return [NSString stringWithFormat:@"cannot load %@: %@", path, error.localizedDescription];
    }
    if (NSClassFromString(@"SPUUpdater") == nil || NSClassFromString(@"SPUStandardUserDriver") == nil) {
        return [NSString stringWithFormat:@"%@ has no SPUUpdater", path];
    }
    return nil;
}

- (instancetype)initWithEvents:(void (^)(HCSparkleEvent event, NSString *text))events
                    skipReason:(NSString *_Nullable (^)(NSString *version))skipReason {
    self = [super init];
    if (self != nil) {
        _events = [events copy];
        _skipReason = [skipReason copy];
    }
    return self;
}

- (void)start {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (self->_delegate != nil) {
            return;
        }
        NSBundle *host = [NSBundle mainBundle];
        HCSparkleDelegate *d = [[HCSparkleDelegate alloc] init];
        // The standard driver is only ever seen when Sparkle needs a person:
        // an update marked critical or informational, or one needing an
        // administrator's password. The release pipeline publishes neither, and
        // the app never starts Sparkle on a bundle it cannot write.
        d->owner = self;
        self->_userDriver = [(id<HCSPUStandardUserDriver>)[NSClassFromString(@"SPUStandardUserDriver") alloc] initWithHostBundle:host delegate:nil];
        self->_updater = [(id<HCSPUUpdater>)[NSClassFromString(@"SPUUpdater") alloc] initWithHostBundle:host applicationBundle:host userDriver:self->_userDriver delegate:d];
        self->_delegate = d;
        NSError *error = nil;
        if (![self->_updater startUpdater:&error]) {
            hcSend(self, HCSparkleStartFailed, error.localizedDescription ?: @"startUpdater failed");
            return;
        }
        hcSend(self, HCSparkleStarted, @"");
    });
}

- (void)installNow {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (self->_delegate == nil || self->_installNow == nil) {
            hcSend(self, HCSparkleInstallFailed, @"Sparkle has no prepared update");
            return;
        }
        self->_installNow();
    });
}

- (void)checkNow {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (self->_delegate == nil) {
            hcSend(self, HCSparkleCheckSkipped, @"Sparkle is not started");
            return;
        }
        if (self->_installNow != nil) {
            hcSend(self, HCSparkleCheckSkipped, @"an update is already prepared");
            return;
        }
        id<HCSPUUpdater> u = self->_updater;
        if ([u sessionInProgress]) {
            hcSend(self, HCSparkleCheckSkipped, @"a check is already running");
            return;
        }
        if (![u canCheckForUpdates]) {
            hcSend(self, HCSparkleCheckSkipped, @"Sparkle cannot check right now");
            return;
        }
        hcSend(self, HCSparkleChecking, @"");
        [u checkForUpdatesInBackground];
    });
}
@end
