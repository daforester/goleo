import UIKit
import WebKit
import UserNotifications
import CoreMotion
import BackgroundTasks
import UniformTypeIdentifiers
import AVFoundation
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
class AppDelegate: UIResponder, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    // No `window` property. Under the scene lifecycle UIKit ignores UIApplicationDelegate's
    // window entirely — SceneDelegate owns it, and a second one here would look like the
    // app's window while nothing ever presented in it.
    var webView: WKWebView?
    let notifier = GoleoNotifier()
    let batteryProvider = GoleoBatteryStatus()
    let wakeLockProvider = GoleoWakeLock()
    let sensorsProvider = GoleoSensors()
    let backgroundProvider = GoleoBackground()
    let clipboardProvider = GoleoClipboardImpl()
    let shareProvider = GoleoShareImpl()
    let dialogsProvider = GoleoDialogs()
    let microphoneProvider = GoleoMicrophone()
    let permissionDelegate = GoleoWebPermissionDelegate()

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        // BGTaskScheduler requires registration before this method returns.
        GoleoBackground.registerTask()

        // WITHOUT THIS, NOTIFICATIONS ARE NEVER SEEN. iOS suppresses a notification
        // whose app is in the foreground unless a delegate implements willPresent and
        // asks for it to be shown. GoleoNotifier.show posts with `trigger: nil`, so
        // every notification fires immediately — i.e. while the app is in front, which
        // is exactly when it was being suppressed. The permission prompt appearing made
        // this look like a delivery failure rather than a presentation one.
        //
        // Apple requires the delegate to be set before the app finishes launching.
        UNUserNotificationCenter.current().delegate = self

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
        GomobileSetDialogsProvider(dialogsProvider)
        GomobileSetMicrophoneProvider(microphoneProvider)

        let port = GomobileStartServer(false)
        let url = URL(string: "http://127.0.0.1:\(port)")!

        let config = WKWebViewConfiguration()
        let userContentController = WKUserContentController()
        config.userContentController = userContentController
        config.allowsInlineMediaPlayback = true
        config.mediaTypesRequiringUserActionForPlayback = []

        // .zero, not UIScreen.main.bounds: this view becomes a view controller's root
        // view, so the window sizes it. UIScreen.main is also deprecated under the scene
        // lifecycle, where a screen is a property of the scene rather than of the app.
        webView = WKWebView(frame: .zero, configuration: config)
        webView?.uiDelegate = permissionDelegate
        webView?.load(URLRequest(url: url))

        // No window is built here. Under the scene lifecycle the window belongs to the
        // scene, and SceneDelegate creates it when the scene connects — right after this
        // method returns. There is deliberately no pre-iOS-13 fallback: the scene manifest
        // makes iOS 13 this shell's floor, and resolveIOSMinVersion refuses a lower
        // deployment target rather than letting it launch to a black screen.
        return true
    }

    /// Names the scene delegate in CODE as well as in Info.plist's
    /// UIApplicationSceneManifest.
    ///
    /// The manifest entry is `$(PRODUCT_MODULE_NAME).SceneDelegate`, which only resolves
    /// because ProcessInfoPlistFile runs with -expandbuildsettings and PRODUCT_NAME is
    /// pinned to GoleoApp in xcodegen.yml. If that expansion ever fails, UIKit cannot find
    /// the class and the app launches to a black screen with one line in the device log.
    /// Setting delegateClass here overrides the string, so the plist is the declaration and
    /// this is the guarantee.
    @available(iOS 13.0, *)
    func application(
        _ application: UIApplication,
        configurationForConnecting connectingSceneSession: UISceneSession,
        options: UIScene.ConnectionOptions
    ) -> UISceneConfiguration {
        let configuration = UISceneConfiguration(
            name: "Default Configuration", sessionRole: connectingSceneSession.role)
        configuration.delegateClass = SceneDelegate.self
        return configuration
    }

    /// Shows notifications that arrive while the app is in the foreground. Returning an
    /// empty set here — which is the DEFAULT when no delegate is set — tells iOS to
    /// discard the presentation silently, which is why notifications appeared to be
    /// permitted but never delivered.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        if #available(iOS 14.0, *) {
            completionHandler([.banner, .list, .sound])
        } else {
            completionHandler([.alert, .sound])
        }
    }

    /// Best-effort shutdown. Under the scene lifecycle this is NOT guaranteed — iOS calls it
    /// when it terminates a running app, but a suspended app is killed without it. That is
    /// acceptable because the process is going away with the server inside it; it is
    /// deliberately not moved to sceneDidDisconnect, which fires when the system releases a
    /// scene the app may well outlive, and would stop the backend under a live app.
    func applicationWillTerminate(_ application: UIApplication) {
        GomobileStopServer()
    }

    /// fileprivate rather than private: SceneDelegate is a separate type in this file and
    /// needs it to build the scene's root.
    fileprivate func makeViewController() -> UIViewController {
        let vc = UIViewController()
        vc.view = webView
        return vc
    }
}

/// Owns the window under the UIScene lifecycle.
///
/// Adopted because the device log said so, in as many words: "`UIScene` lifecycle will soon
/// be required. Failure to adopt will result in an assert in the future." (iPhone, iOS 26,
/// 2026-08-10). The app-delegate window still worked at that point — this is the migration,
/// not a fix for a break.
///
/// The split is deliberately minimal. Everything that is per-APP stays in AppDelegate:
/// starting the Go server, registering providers, the BGTaskScheduler registration (which
/// MUST happen before didFinishLaunching returns), the notification delegate. Only the
/// window, which is per-SCENE, moves here. The WebView is still created in
/// didFinishLaunching and simply gets attached when the scene connects, so the server start
/// and the page load keep the ordering they were device-verified with.
@available(iOS 13.0, *)
class SceneDelegate: UIResponder, UIWindowSceneDelegate {
    var window: UIWindow?

    func scene(
        _ scene: UIScene,
        willConnectTo session: UISceneSession,
        options connectionOptions: UIScene.ConnectionOptions
    ) {
        guard let windowScene = scene as? UIWindowScene else { return }
        guard let app = UIApplication.shared.delegate as? AppDelegate else { return }

        let window = UIWindow(windowScene: windowScene)
        window.rootViewController = app.makeViewController()
        window.makeKeyAndVisible()
        self.window = window
        // Anything that presents a sheet or an alert resolves its presenter through
        // GoleoUI, which needs this window — see GoleoUI for why asking UIApplication
        // for it does not work.
        GoleoUI.window = window
    }
}

/// Resolves the view controller that sheets and alerts are presented from.
///
/// This exists because `UIApplication.shared.windows` does not work here. It has been
/// deprecated since iOS 15 and returns an empty array under the scene lifecycle, so the
/// share sheet resolved a nil root view controller and `present` did nothing at all — no
/// sheet, no exception, and no error path back to Go, since ShareProvider.share returns
/// void.
///
/// The window the app actually created is therefore handed over directly — by SceneDelegate
/// when the scene connects — and the scene lookup below is only a fallback. It stays a
/// fallback rather than becoming the primary now that scenes are adopted: it is what answers
/// for the sliver between the server starting and the scene connecting, and for any future
/// window this file does not create.
enum GoleoUI {
    static weak var window: UIWindow?

    static func topViewController() -> UIViewController? {
        var top = window?.rootViewController
        if top == nil {
            top = UIApplication.shared.connectedScenes
                .compactMap { $0 as? UIWindowScene }
                .flatMap { $0.windows }
                .first(where: { $0.isKeyWindow })?
                .rootViewController
        }
        // Presenting on a controller that is already presenting throws, so walk to the
        // top of the chain — a dialog opened from a share sheet is a real sequence.
        while let presented = top?.presentedViewController {
            top = presented
        }
        return top
    }

    /// Anchors a popover so iPad does not raise
    /// "UIPopoverPresentationController should have a non-nil sourceView or barButtonItem".
    /// iPhone ignores this; iPad crashes without it.
    static func anchorAsPopover(_ controller: UIViewController, in presenter: UIViewController) {
        guard let popover = controller.popoverPresentationController else { return }
        popover.sourceView = presenter.view
        popover.sourceRect = CGRect(
            x: presenter.view.bounds.midX, y: presenter.view.bounds.midY, width: 0, height: 0)
        popover.permittedArrowDirections = []
    }
}

/// Grants camera/mic/location permission requests from web content
/// (getUserMedia, navigator.geolocation) so the browser-API fallbacks used
/// by the JS bridge work in the WKWebView. The corresponding
/// NS*UsageDescription strings must be present in Info.plist or iOS denies
/// (and, for the first prompt, terminates the app) automatically.
class GoleoWebPermissionDelegate: NSObject, WKUIDelegate {
    /// Whether a permission request came from the app's own UI.
    ///
    /// Both callbacks below used to ignore `origin` and grant unconditionally, so ANY page
    /// the WebView reached was silently handed the camera, microphone and location — and
    /// nothing restricts navigation, so an app that links out, or renders a link in user
    /// content, exposes exactly that. Android checked its origin; iOS did not.
    ///
    /// Compares the HOST for equality rather than matching a prefix: `127.0.0.1.evil.com`
    /// is an ordinary registrable domain, and a `hasPrefix("http://127.0.0.1")` test
    /// accepts it. The port is deliberately not checked — goleo's server falls forward to
    /// the next free port when its configured one is taken, so pinning one would deny the
    /// app's own UI. Same shape as devOriginAllowed() in runtime/server.go.
    private func isAppOrigin(_ origin: WKSecurityOrigin) -> Bool {
        // `protocol` is a Swift keyword, hence the backticks.
        guard origin.`protocol` == "http" else { return false }
        return ["127.0.0.1", "localhost", "::1"].contains(origin.host)
    }

    @available(iOS 15.0, *)
    func webView(
        _ webView: WKWebView,
        requestMediaCapturePermissionFor origin: WKSecurityOrigin,
        initiatedByFrame frame: WKFrameInfo,
        type: WKMediaCaptureType,
        decisionHandler: @escaping (WKPermissionDecision) -> Void
    ) {
        decisionHandler(isAppOrigin(origin) ? .grant : .deny)
    }

    @available(iOS 15.4, *)
    func webView(
        _ webView: WKWebView,
        requestGeolocationPermissionFor origin: WKSecurityOrigin,
        initiatedByFrame frame: WKFrameInfo,
        decisionHandler: @escaping (WKPermissionDecision) -> Void
    ) {
        decisionHandler(isAppOrigin(origin) ? .grant : .deny)
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
            guard let presenter = GoleoUI.topViewController() else {
                NSLog("goleo: share sheet not shown — no view controller to present from")
                return
            }
            GoleoUI.anchorAsPopover(vc, in: presenter)
            presenter.present(vc, animated: true)
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
        // The completion handler used to be dropped. A rejected notification — an
        // unauthorized app, a malformed request — then failed in total silence, which is
        // the hardest version of this bug to diagnose from a device.
        UNUserNotificationCenter.current().add(request) { error in
            if let error = error {
                NSLog("goleo: notification not delivered: \(error.localizedDescription)")
            }
        }
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

/// Reports and requests microphone permission via AVAudioSession. Implements the
/// gomobile-generated MicrophoneProvider interface.
///
/// Recording is the WebView's job (getUserMedia + MediaRecorder); only the permission state
/// needs a native call, because a web page cannot ask about it without starting a capture.
///
/// Method shapes match GoleoNotifier's — a lone Bool and a lone String, no throwing. A
/// provider method returning a value AND an error does not bind to Swift at all; see the
/// note on GoleoDialogs.
class GoleoMicrophone: NSObject, GomobileMicrophoneProviderProtocol {
    func permissionGranted() -> Bool {
        if #available(iOS 17.0, *) {
            return AVAudioApplication.shared.recordPermission == .granted
        }
        return AVAudioSession.sharedInstance().recordPermission == .granted
    }

    func requestPermission() -> String {
        if permissionGranted() {
            return "granted"
        }
        var status = "default"
        let semaphore = DispatchSemaphore(value: 0)
        let handler: (Bool) -> Void = { allowed in
            status = allowed ? "granted" : "denied"
            semaphore.signal()
        }
        if #available(iOS 17.0, *) {
            AVAudioApplication.requestRecordPermission(completionHandler: handler)
        } else {
            AVAudioSession.sharedInstance().requestRecordPermission(handler)
        }
        // Bounded wait, exactly as GoleoNotifier.requestPermission does it: while the system
        // prompt is on screen there is no answer yet, so report "default" and let the app
        // ask again rather than blocking a bridge handler on the user.
        _ = semaphore.wait(timeout: .now() + 0.5)
        return status
    }
}

/// Shows native dialogs and file pickers via UIKit. Implements the gomobile-generated
/// DialogsProvider interface.
///
/// Method shapes, which are load-bearing and were got wrong once: each Go method is
/// `XxxJSON(optsJSON string) string` — a lone string result, NO error — so gobind emits
/// `-(NSString* _Nonnull)xxxJSON:(NSString* _Nullable)optsJSON` and Swift sees
/// `func xxxJSON(_ optsJSON: String?) -> String`. Same shape as
/// `GoleoClipboardImpl.readText`, which has compiled here for several releases.
///
/// Do NOT "improve" this by giving the Go methods an `(string, error)` result. gobind then
/// emits a **_Nonnull** NSString return alongside `error:(NSError**)`, Swift will not turn
/// that into a throwing method (a non-optional result cannot signal failure), and NO Swift
/// signature conforms — the build fails with "type 'GoleoDialogs' does not conform to
/// protocol 'GomobileDialogsProviderProtocol'". That shipped in 0.10.7.
///
/// Failures therefore travel inside the returned JSON as {"error":"..."}; see
/// backend/gomobile/dialogs.go for the envelope.
///
/// Every method BLOCKS its calling thread until the user answers. That is safe only
/// because the Go bridge handlers run on goroutines; a call that finds itself on the main
/// thread reports an error rather than deadlocking the UI behind a dialog that can never
/// appear.
class GoleoDialogs: NSObject, GomobileDialogsProviderProtocol {
    // UIDocumentPickerViewController holds its delegate WEAKLY, so it has to be owned
    // here. A delegate left to the local scope is deallocated before the user picks
    // anything, its callbacks never fire, and the Go side waits forever.
    private var pickerDelegate: GoleoDocumentPickerDelegate?

    // Serialises presentation. Only one modal can be on screen at a time anyway, and
    // pickerDelegate above is a single slot: two concurrent openFile calls would have the
    // second overwrite the first's delegate, so the first would never be told anything and
    // would block forever. Concurrent calls are ordinary rather than exotic — the bridge
    // runs every invoke on its own goroutine (runtime/websocket.go), so a frontend that
    // fires two dialogs without awaiting the first gets exactly this.
    private let serial = DispatchSemaphore(value: 1)

    private func fail(_ message: String) -> NSError {
        NSError(domain: "Goleo", code: 2, userInfo: [NSLocalizedDescriptionKey: message])
    }

    /// Runs `body` on the main queue with a resolved presenter and blocks until it
    /// reports a result. There is deliberately no timeout — a dialog is answered when the
    /// user answers it, and a timeout would report a cancellation they never made.
    private func presentBlocking(
        _ body: @escaping (UIViewController, @escaping (String, Error?) -> Void) -> Void
    ) throws -> String {
        // Checked BEFORE taking the lock: blocking the main thread here would freeze the
        // very thread that has to draw the dialog, and doing it while holding the lock
        // would take every subsequent dialog down with it.
        if Thread.isMainThread {
            throw fail("goleo dialogs cannot be shown from the main thread")
        }
        serial.wait()
        defer { serial.signal() }

        let semaphore = DispatchSemaphore(value: 0)
        var result = ""
        var failure: Error?
        var settled = false
        DispatchQueue.main.async {
            let finish: (String, Error?) -> Void = { value, error in
                // An alert can report twice (an action AND a dismissal). Signalling the
                // semaphore twice would release some later, unrelated wait.
                if settled { return }
                settled = true
                result = value
                failure = error
                semaphore.signal()
            }
            guard let presenter = GoleoUI.topViewController() else {
                finish("", self.fail("no view controller available to present a dialog"))
                return
            }
            body(presenter, finish)
        }
        semaphore.wait()
        if let failure = failure { throw failure }
        return result
    }

    func showMessageJSON(_ optsJSON: String?) -> String {
        let opts = GoleoDialogs.decode(optsJSON)
        var buttons = opts["buttons"] as? [String] ?? []
        if buttons.isEmpty { buttons = ["OK"] }
        do {
            return GoleoDialogs.reply(value: try presentBlocking { presenter, finish in
            let alert = UIAlertController(
                title: opts["title"] as? String,
                message: opts["message"] as? String,
                preferredStyle: .alert)
            for (index, button) in buttons.enumerated() {
                // The last of several buttons is the dismissive one by convention, and
                // .cancel styling is what makes that readable on iOS.
                let style: UIAlertAction.Style =
                    (buttons.count > 1 && index == buttons.count - 1) ? .cancel : .default
                alert.addAction(UIAlertAction(title: button, style: style) { _ in
                    finish(button, nil)
                })
            }
            GoleoUI.anchorAsPopover(alert, in: presenter)
            presenter.present(alert, animated: true)
            })
        } catch {
            return GoleoDialogs.reply(error: error.localizedDescription)
        }
    }

    func showPromptJSON(_ optsJSON: String?) -> String {
        let opts = GoleoDialogs.decode(optsJSON)
        let defaultValue = opts["defaultValue"] as? String ?? ""
        do {
            return GoleoDialogs.reply(value: try presentBlocking { presenter, finish in
            let alert = UIAlertController(
                title: opts["title"] as? String,
                message: opts["message"] as? String,
                preferredStyle: .alert)
            alert.addTextField { field in field.text = defaultValue }
            // Empty means cancelled. A cancelled prompt and an empty answer are
            // indistinguishable here, exactly as they are on Windows/macOS/Linux.
            alert.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in
                finish("", nil)
            })
            alert.addAction(UIAlertAction(title: "OK", style: .default) { _ in
                finish(alert.textFields?.first?.text ?? "", nil)
            })
            presenter.present(alert, animated: true)
            })
        } catch {
            return GoleoDialogs.reply(error: error.localizedDescription)
        }
    }

    func openFileJSON(_ optsJSON: String?) -> String {
        let opts = GoleoDialogs.decode(optsJSON)
        let multiple = opts["multiple"] as? Bool ?? false
        let types = GoleoDialogs.contentTypes(from: opts["filters"])
        // The delegate is only needed for the duration of the picker; presentBlocking has
        // already returned by the time this runs, so releasing it here cannot cut short a
        // picker that is still on screen.
        defer { pickerDelegate = nil }
        do {
            // The picker's callback builds the final envelope directly, so there is no
            // array to re-encode on the way out.
            return try presentBlocking { presenter, finish in
            // asCopy: true copies the chosen file into this app's temporary directory and
            // hands back a plain readable path. The alternative is a security-scoped URL,
            // which the Go fs plugin could not open: it takes paths and knows nothing
            // about startAccessingSecurityScopedResource. RegisterDialogs then calls
            // GrantFSPath on whatever comes back, so reading it needs no extra config.
            let picker = UIDocumentPickerViewController(forOpeningContentTypes: types, asCopy: true)
            picker.allowsMultipleSelection = multiple
            let delegate = GoleoDocumentPickerDelegate { urls in
                finish(GoleoDialogs.reply(paths: urls.map { $0.path }), nil)
            }
            self.pickerDelegate = delegate
            picker.delegate = delegate
            GoleoUI.anchorAsPopover(picker, in: presenter)
            presenter.present(picker, animated: true)
            }
        } catch {
            return GoleoDialogs.reply(error: error.localizedDescription)
        }
    }

    /// Asks for a FILENAME and returns a path inside the app's Documents directory.
    ///
    /// iOS has no "choose a destination, then write to it" primitive: the document picker
    /// exports a file that already exists, while the caller here needs a path to write to
    /// afterwards. Documents is the platform-idiomatic answer, and Info.plist publishes it
    /// to the Files app (UIFileSharingEnabled + LSSupportsOpeningDocumentsInPlace) so the
    /// saved file is reachable once written.
    func saveFileJSON(_ optsJSON: String?) -> String {
        let opts = GoleoDialogs.decode(optsJSON)
        let suggested = (((opts["defaultPath"] as? String) ?? "") as NSString).lastPathComponent
        let name: String
        do {
            name = try presentBlocking { presenter, finish in
            let alert = UIAlertController(
                title: opts["title"] as? String ?? "Save File",
                message: nil,
                preferredStyle: .alert)
            alert.addTextField { field in
                field.text = suggested
                field.placeholder = "File name"
            }
            alert.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in
                finish("", nil)
            })
            alert.addAction(UIAlertAction(title: "Save", style: .default) { _ in
                finish(alert.textFields?.first?.text ?? "", nil)
            })
            presenter.present(alert, animated: true)
            }
        } catch {
            return GoleoDialogs.reply(error: error.localizedDescription)
        }
        if name.isEmpty {
            return GoleoDialogs.reply(value: "") // cancelled
        }
        // Basename only. A typed name containing "/" would otherwise name a path in a
        // subdirectory that does not exist, so the write this method exists to enable
        // would fail — and the Android shell already takes the basename, so without this
        // the two platforms disagreed about the same input.
        let safeName = (name as NSString).lastPathComponent
        if safeName.isEmpty {
            return GoleoDialogs.reply(value: "")
        }
        return GoleoDialogs.reply(
            value: GoleoDialogs.documentsDirectory().appendingPathComponent(safeName).path)
    }

    /// Returns the app's Documents directory, without a picker.
    ///
    /// This is the honest answer, not a shortcut. Neither mobile platform can hand back a
    /// plain path for an arbitrary user-chosen directory: iOS's folder picker yields a
    /// security-scoped URL and Android's ACTION_OPEN_DOCUMENT_TREE yields a tree URI, and
    /// the Go fs plugin — which takes paths — can read neither. A sandboxed app's own
    /// document directory is the only directory it can both name and actually read, so
    /// both shells return that rather than a path that would fail on first use.
    func selectFolderJSON(_ optsJSON: String?) -> String {
        return GoleoDialogs.reply(value: GoleoDialogs.documentsDirectory().path)
    }

    private static func documentsDirectory() -> URL {
        return FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
    }

    private static func decode(_ json: String?) -> [String: Any] {
        guard let data = json?.data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else { return [:] }
        return object
    }

    /// The reply envelope the Go adapter decodes — see backend/gomobile/dialogs.go.
    /// Failures travel in here because the bound methods cannot return an error.
    private static func reply(value: String) -> String {
        return encodeReply(["value": value])
    }

    private static func reply(paths: [String]) -> String {
        return encodeReply(["paths": paths])
    }

    private static func reply(error: String) -> String {
        // Never empty: an empty reply reads as a cancellation on the Go side, which would
        // turn a real failure into "the user dismissed it".
        return encodeReply(["error": error.isEmpty ? "the dialog failed" : error])
    }

    private static func encodeReply(_ object: [String: Any]) -> String {
        guard let data = try? JSONSerialization.data(withJSONObject: object),
              let text = String(data: data, encoding: .utf8)
        else { return "{\"error\":\"goleo: could not encode the dialog reply\"}" }
        return text
    }

    /// Maps FileDialogOptions.Filters patterns ("*.txt") onto UTTypes. A pattern that does
    /// not resolve is dropped rather than guessed at, and if none resolve the picker
    /// allows any file — better than presenting one that can select nothing.
    private static func contentTypes(from raw: Any?) -> [UTType] {
        guard let filters = raw as? [[String: Any]] else { return [.item] }
        var types: [UTType] = []
        for filter in filters {
            for pattern in (filter["patterns"] as? [String]) ?? [] {
                let ext = pattern
                    .replacingOccurrences(of: "*", with: "")
                    .replacingOccurrences(of: ".", with: "")
                    .trimmingCharacters(in: .whitespaces)
                if ext.isEmpty { continue }
                if let type = UTType(filenameExtension: ext) { types.append(type) }
            }
        }
        return types.isEmpty ? [.item] : types
    }
}

/// Bridges UIDocumentPickerViewController's delegate callbacks to a single completion.
/// Cancellation reports an empty selection rather than an error — the Go contract is that
/// no paths means the user cancelled, which is what the desktop implementations return.
class GoleoDocumentPickerDelegate: NSObject, UIDocumentPickerDelegate {
    private let onFinish: ([URL]) -> Void
    private var finished = false

    init(onFinish: @escaping ([URL]) -> Void) {
        self.onFinish = onFinish
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        complete(urls)
    }

    func documentPickerWasCancelled(_ controller: UIDocumentPickerViewController) {
        complete([])
    }

    private func complete(_ urls: [URL]) {
        if finished { return }
        finished = true
        onFinish(urls)
    }
}
