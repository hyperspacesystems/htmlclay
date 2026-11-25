#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

typedef NS_ENUM(NSInteger, HCSparkleEvent) {
    HCSparkleStarted = 1,
    HCSparkleStartFailed,
    HCSparkleReady,
    HCSparkleAborted,
    HCSparkleInstallFailed,
    HCSparkleCheckSkipped,
    HCSparkleChecking,
    HCSparkleUpToDate,
    HCSparkleCheckEnded,
    HCSparkleFound
};

@interface HCSparkleBridge : NSObject
+ (nullable NSString *)loadFrameworkAtPath:(NSString *)path;
- (instancetype)initWithEvents:(void (^)(HCSparkleEvent event, NSString *text))events
                    skipReason:(NSString *_Nullable (^)(NSString *version))skipReason;
- (void)start;
- (void)installNow;
- (void)checkNow;
/// Whether Sparkle checks for and installs updates by itself, which Sparkle
/// keeps in the app's defaults. Main thread only; YES until Sparkle starts.
@property (nonatomic) BOOL automatic;
@end

NS_ASSUME_NONNULL_END
