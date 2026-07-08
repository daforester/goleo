import UIKit
import WebKit
import UserNotifications
import Goleo

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    var webView: WKWebView?
    let notifier = GoleoNotifier()
    let permissionDelegate = GoleoWebPermissionDelegate()

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        Goleo.setNotifier(notifier)

        let port = Goleo.startServer()
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
