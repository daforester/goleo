import UIKit
import WebKit
import UserNotifications
import CoreMotion
import BackgroundTasks
import Goleo

// BGTaskScheduler identifiers must be declared statically in Info.plist's
// BGTaskSchedulerPermittedIdentifiers, and registering one that is NOT in that list
// raises an NSException — from registerTask() below, which runs first thing in
// didFinishLaunching, so the app crashes on launch.
//
// Info.plist permits $(PRODUCT_BUNDLE_IDENTIFIER).sync, and PRODUCT_BUNDLE_IDENTIFIER is
// the IOS bundle id. This used the ANDROID package name, which was harmless only while the
// iOS build reused the Android id for everything. Making mobile.ios.bundle_identifier take
// effect (0.10.2) is what turned it into a crash for any project that sets one. IOSBundleID
// falls back to PackageName, so the default scaffold is unchanged.
let backgroundSyncTaskID = "{{.IOSBundleID}}.sync"

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    var webView: WKWebView?
    let notifier = GoleoNotifier()
    let batteryProvider = GoleoBatteryStatus()
    let wakeLockProvider = GoleoWakeLock()
    let sensorsProvider = GoleoSensors()
    let backgroundProvider = GoleoBackground()
    let clipboardProvider = GoleoClipboardImpl()
    let shareProvider = GoleoShareImpl()
    let permissionDelegate = GoleoWebPermissionDelegate()

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        // BGTaskScheduler requires registration before this method returns.
        GoleoBackground.registerTask()

        // Go's os.UserConfigDir/os.UserHomeDir (used by the FS feature's
        // AppDataDir/HomeDir) need $HOME, which the gomobile host process
        // never sets on its own — must run before startServer.
        GomobileSetHomeDir(NSHomeDirectory())
        GomobileSetNotifier(notifier)
        GomobileSetBatteryProvider(batteryProvider)
        GomobileSetWakeLockProvider(wakeLockProvider)
        GomobileSetSensorsProvider(sensorsProvider)
        GomobileSetBackgroundProvider(backgroundProvider)
        GomobileSetClipboardProvider(clipboardProvider)
        GomobileSetShareProvider(shareProvider)

        let port = GomobileStartServer(false)
        let url = URL(string: "http://127.0.0.1:\(port)")!

        let config = WKWebViewConfiguration()
        let userContentController = WKUserContentController()
        config.userContentController = userContentController
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []

        webView = WKWebView(frame: UIScreen.main.bounds, configuration: config)
        webView?.uiDelegate = permissionDelegate
        webView?.load(URLRequest(url: url))

        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = makeViewController()
        window?.makeKeyAndVisible()

        return true
    }

    func applicationWillTerminate(_ application: UIApplication) {
        GomobileStopServer()
    }

    private func makeViewController() -> UIViewController {
        let vc = UIViewController()
        vc.view = webView
        return vc
    }
}

/// Grants camera/mic/location permission requests from web content
/// (getUserMedia, navigator.geolocation) so the browser-API fallbacks used
/// by the JS bridge work in the WKWebView. The corresponding
/// NS*UsageDescription strings must be present in Info.plist or iOS denies
/// (and, for the first prompt, terminates the app) automatically.
class GoleoWebPermissionDelegate: NSObject, WKUIDelegate {
    @available(iOS 15.0, *)
    func webView(
        _ webView: WKWebView,
        requestMediaCapturePermissionFor origin: WKSecurityOrigin,
        initiatedByFrame frame: WKFrameInfo,
        type: WKMediaCaptureType,
        decisionHandler: @escaping (WKPermissionDecision) -> Void
    ) {
        decisionHandler(.grant)
    }

    @available(iOS 15.4, *)
    func webView(
        _ webView: WKWebView,
        requestGeolocationPermissionFor origin: WKSecurityOrigin,
        initiatedByFrame frame: WKFrameInfo,
        decisionHandler: @escaping (WKPermissionDecision) -> Void
    ) {
        decisionHandler(.grant)
    }
}

/// Reports real battery state via UIDevice. Implements the
/// gomobile-generated BatteryProvider interface.
class GoleoBatteryStatus: NSObject, GomobileBatteryProviderProtocol {
    override init() {
        super.init()
        UIDevice.current.isBatteryMonitoringEnabled = true
    }

    func level() -> Double {
        let level = UIDevice.current.batteryLevel
        return level < 0 ? -1 : Double(level)
    }

    func charging() -> Bool {
        let state = UIDevice.current.batteryState
        return state == .charging || state == .full
    }

    // iOS has no public API for time-to-full/time-to-empty estimates.
    func chargingTime() -> Double { -1 }
    func dischargingTime() -> Double { -1 }
}

/// Keeps the screen awake via UIApplication's idle timer. Implements the
/// gomobile-generated WakeLockProvider interface.
class GoleoWakeLock: NSObject, GomobileWakeLockProviderProtocol {
    func request(_ typeName: String?) throws {
        DispatchQueue.main.async {
            UIApplication.shared.isIdleTimerDisabled = true
        }
    }

    func release() throws {
        DispatchQueue.main.async {
            UIApplication.shared.isIdleTimerDisabled = false
        }
    }
}

/// Reads/writes the system clipboard via UIPasteboard. Implements the
/// gomobile-generated ClipboardProvider interface.
/// Method shapes checked against the generated Gomobile.objc.h:
/// `-(NSString*)readText` and `-(void)writeText:(NSString*)text`.
class GoleoClipboardImpl: NSObject, GomobileClipboardProviderProtocol {
    func readText() -> String {
        if Thread.isMainThread {
            return UIPasteboard.general.string ?? ""
        }
        var result = ""
        DispatchQueue.main.sync {
            result = UIPasteboard.general.string ?? ""
        }
        return result
    }

    func writeText(_ text: String?) {
        DispatchQueue.main.async {
            UIPasteboard.general.string = text ?? ""
        }
    }
}

/// Opens the system share sheet via UIActivityViewController. Implements the
/// gomobile-generated ShareProvider interface.
/// Checked against the header: `-(void)share:(NSString*)title
/// text:(NSString*)text url:(NSString*)url`, so Swift sees share(_:text:url:).
class GoleoShareImpl: NSObject, GomobileShareProviderProtocol {
    func share(_ title: String?, text: String?, url: String?) {
        DispatchQueue.main.async {
            var items: [Any] = []
            if let t = text, !t.isEmpty { items.append(t) }
            if let u = url, !u.isEmpty, let link = URL(string: u) { items.append(link) }
            if items.isEmpty, let t = title, !t.isEmpty { items.append(t) }
            let vc = UIActivityViewController(activityItems: items, applicationActivities: nil)
            let root = UIApplication.shared.windows.first(where: { $0.isKeyWindow })?.rootViewController
            root?.present(vc, animated: true)
        }
    }
}

/// Streams accelerometer/gyroscope/magnetometer readings from CoreMotion to
/// the frontend via GomobileEmitSensorReading, which turns into a
/// goleo:sensorReading event (see SensorsDemo.vue). Implements the
/// gomobile-generated SensorsProvider interface.
///
/// NOTE on naming, because two different names are in play and neither is
/// guessable from this file: the Swift MODULE is `Goleo` (gomobile titlecases
/// the -o basename, Goleo.xcframework) but every SYMBOL carries the titlecased
/// Go PACKAGE name, `Gomobile`. So it is `import Goleo` plus
/// `GomobileSetHomeDir(...)`. Package-level Go funcs become C functions, which
/// take no argument labels: `GomobileEmitSensorReading(t, x, y, z, ts)`, not
/// `emitSensorReading(x:y:z:timestamp:)`. Each Go interface generates both a
/// protocol and a same-named wrapper class, so Swift appends `Protocol` to the
/// protocol name: `GomobileSensorsProviderProtocol`. Verified against the
/// generated Gomobile.objc.h on a macos-14 runner — mobile-verify prints it.
class GoleoSensors: NSObject, GomobileSensorsProviderProtocol {
    private let motionManager = CMMotionManager()

    private func fail(_ message: String) -> NSError {
        NSError(domain: "Goleo", code: 1, userInfo: [NSLocalizedDescriptionKey: message])
    }

    func startSensor(_ sensorType: String?) throws {
        let now = { Int64(Date().timeIntervalSince1970 * 1000) }
        switch sensorType {
        case "accelerometer":
            guard motionManager.isAccelerometerAvailable else { throw fail("accelerometer not available") }
            motionManager.accelerometerUpdateInterval = 1.0 / 60.0
            motionManager.startAccelerometerUpdates(to: .main) { data, _ in
                guard let a = data?.acceleration else { return }
                GomobileEmitSensorReading("accelerometer", a.x, a.y, a.z, now())
            }
        case "gyroscope":
            guard motionManager.isGyroAvailable else { throw fail("gyroscope not available") }
            motionManager.gyroUpdateInterval = 1.0 / 60.0
            motionManager.startGyroUpdates(to: .main) { data, _ in
                guard let r = data?.rotationRate else { return }
                GomobileEmitSensorReading("gyroscope", r.x, r.y, r.z, now())
            }
        case "magnetometer":
            guard motionManager.isMagnetometerAvailable else { throw fail("magnetometer not available") }
            motionManager.magnetometerUpdateInterval = 1.0 / 60.0
            motionManager.startMagnetometerUpdates(to: .main) { data, _ in
                guard let m = data?.magneticField else { return }
                GomobileEmitSensorReading("magnetometer", m.x, m.y, m.z, now())
            }
        default:
            throw fail("unsupported sensor: \(sensorType ?? "")")
        }
    }

    func stopSensor(_ sensorType: String?) throws {
        switch sensorType {
        case "accelerometer": motionManager.stopAccelerometerUpdates()
        case "gyroscope": motionManager.stopGyroUpdates()
        case "magnetometer": motionManager.stopMagnetometerUpdates()
        default: break
        }
    }
}

/// Runs a registered sync tag as a BGProcessingTask, deferred until
/// connectivity is available; the task handler reports back via
/// GomobileEmitBackgroundSync when it actually runs. Implements the
/// gomobile-generated BackgroundProvider interface.
///
/// BGTaskScheduler identifies tasks by a fixed identifier (declared in
/// Info.plist's BGTaskSchedulerPermittedIdentifiers), not a dynamic tag, so
/// the tag is stashed in UserDefaults and read back when the task fires.
class GoleoBackground: NSObject, GomobileBackgroundProviderProtocol {
    static func registerTask() {
        BGTaskScheduler.shared.register(forTaskWithIdentifier: backgroundSyncTaskID, using: nil) { task in
            let tag = UserDefaults.standard.string(forKey: "goleo.pendingSyncTag") ?? ""
            GomobileEmitBackgroundSync(tag)
            task.setTaskCompleted(success: true)
        }
    }

    func registerSync(_ tag: String?) throws {
        UserDefaults.standard.set(tag ?? "", forKey: "goleo.pendingSyncTag")
        let request = BGProcessingTaskRequest(identifier: backgroundSyncTaskID)
        request.requiresNetworkConnectivity = true
        try BGTaskScheduler.shared.submit(request)
    }

    // BGTaskScheduler needs no runtime permission to schedule work.
    func getPermission() -> Bool { true }

    func requestPermission() throws {}
}

/// Delivers notifications from the Go runtime through UNUserNotificationCenter.
/// Implements the gomobile-generated Notifier interface.
class GoleoNotifier: NSObject, GomobileNotifierProtocol {
    func show(_ title: String?, body: String?) {
        let content = UNMutableNotificationContent()
        content.title = title ?? "Goleo"
        content.body = body ?? ""
        content.sound = .default

        let request = UNNotificationRequest(
            identifier: UUID().uuidString,
            content: content,
            trigger: nil
        )
        UNUserNotificationCenter.current().add(request)
    }

    func permissionGranted() -> Bool {
        var granted = false
        let semaphore = DispatchSemaphore(value: 0)
        UNUserNotificationCenter.current().getNotificationSettings { settings in
            granted = settings.authorizationStatus == .authorized
                || settings.authorizationStatus == .provisional
            semaphore.signal()
        }
        semaphore.wait()
        return granted
    }

    func requestPermission() -> String {
        if permissionGranted() {
            return "granted"
        }
        var status = "default"
        let semaphore = DispatchSemaphore(value: 0)
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound]) { granted, _ in
            status = granted ? "granted" : "denied"
            semaphore.signal()
        }
        // Bounded wait: if the system dialog is showing, report "default"
        // and let the app query again later.
        _ = semaphore.wait(timeout: .now() + 0.5)
        return status
    }
}
