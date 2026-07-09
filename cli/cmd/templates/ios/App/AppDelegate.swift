import UIKit
import WebKit
import UserNotifications
import CoreMotion
import BackgroundTasks
import Goleo

// BGTaskScheduler identifiers must be declared statically in Info.plist's
// BGTaskSchedulerPermittedIdentifiers, so this can't be templated further —
// keep it in sync with Info.plist if you change it.
let backgroundSyncTaskID = "{{.PackageName}}.sync"

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    var webView: WKWebView?
    let notifier = GoleoNotifier()
    let batteryProvider = GoleoBatteryStatus()
    let wakeLockProvider = GoleoWakeLock()
    let sensorsProvider = GoleoSensors()
    let backgroundProvider = GoleoBackground()
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
        Goleo.setHomeDir(NSHomeDirectory())
        Goleo.setNotifier(notifier)
        Goleo.setBatteryProvider(batteryProvider)
        Goleo.setWakeLockProvider(wakeLockProvider)
        Goleo.setSensorsProvider(sensorsProvider)
        Goleo.setBackgroundProvider(backgroundProvider)

        let port = Goleo.startServer(devMode: false)
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
        Goleo.stopServer()
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
class GoleoBatteryStatus: NSObject, GoleoBatteryProviderProtocol {
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
class GoleoWakeLock: NSObject, GoleoWakeLockProviderProtocol {
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

/// Streams accelerometer/gyroscope/magnetometer readings from CoreMotion to
/// the frontend via Goleo.emitSensorReading, which turns into a
/// goleo:sensorReading event (see SensorsDemo.vue). Implements the
/// gomobile-generated SensorsProvider interface.
///
/// NOTE: the exact Swift argument-label syntax gomobile generates for a
/// multi-parameter Go function like EmitSensorReading is unverified here
/// (no Xcode/macOS available) — if `Goleo.emitSensorReading(...)` doesn't
/// compile as written, check the generated Goleo.swiftmodule/header for the
/// real label names and adjust.
class GoleoSensors: NSObject, GoleoSensorsProviderProtocol {
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
                Goleo.emitSensorReading("accelerometer", x: a.x, y: a.y, z: a.z, timestamp: now())
            }
        case "gyroscope":
            guard motionManager.isGyroAvailable else { throw fail("gyroscope not available") }
            motionManager.gyroUpdateInterval = 1.0 / 60.0
            motionManager.startGyroUpdates(to: .main) { data, _ in
                guard let r = data?.rotationRate else { return }
                Goleo.emitSensorReading("gyroscope", x: r.x, y: r.y, z: r.z, timestamp: now())
            }
        case "magnetometer":
            guard motionManager.isMagnetometerAvailable else { throw fail("magnetometer not available") }
            motionManager.magnetometerUpdateInterval = 1.0 / 60.0
            motionManager.startMagnetometerUpdates(to: .main) { data, _ in
                guard let m = data?.magneticField else { return }
                Goleo.emitSensorReading("magnetometer", x: m.x, y: m.y, z: m.z, timestamp: now())
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
/// Goleo.emitBackgroundSync when it actually runs. Implements the
/// gomobile-generated BackgroundProvider interface.
///
/// BGTaskScheduler identifies tasks by a fixed identifier (declared in
/// Info.plist's BGTaskSchedulerPermittedIdentifiers), not a dynamic tag, so
/// the tag is stashed in UserDefaults and read back when the task fires.
class GoleoBackground: NSObject, GoleoBackgroundProviderProtocol {
    static func registerTask() {
        BGTaskScheduler.shared.register(forTaskWithIdentifier: backgroundSyncTaskID, using: nil) { task in
            let tag = UserDefaults.standard.string(forKey: "goleo.pendingSyncTag") ?? ""
            Goleo.emitBackgroundSync(tag)
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
class GoleoNotifier: NSObject, GoleoNotifierProtocol {
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
