#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

typedef NS_ENUM(NSInteger, HCSparkleEvent) {
    HCSparkleStarted = 1,
    HCSparkleStartFailed,
    HCSparkleReady,
    HCSparkleAborted,
    HCSparkleInstallFailed,
    HCSparkleCheckSkipped
};

@interface HCSparkleBridge : NSObject
+ (nullable NSString *)loadFrameworkAtPath:(NSString *)path;
- (instancetype)initWithEvents:(void (^)(HCSparkleEvent event, NSString *text))events
                    skipReason:(NSString *_Nullable (^)(NSString *version))skipReason;
- (void)start;
- (void)installNow;
- (void)checkNow;
@end

NS_ASSUME_NONNULL_END
