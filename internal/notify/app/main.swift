import AppKit
import UserNotifications

let notAllowed: Int32 = 2

// Exit on UserNotifications' queue would run the atexit handlers there while
// AppKit's run loop carries on on the main thread.
func finish(_ code: Int32) {
    DispatchQueue.main.async { exit(code) }
}

// macOS may answer an app whose notifications the user turned off with
// notificationsNotAllowed rather than a plain no.
func finish(_ error: Error) {
    warn(error.localizedDescription)
    if let error = error as? UNError, error.code == .notificationsNotAllowed {
        finish(notAllowed)
    } else {
        finish(EXIT_FAILURE)
    }
}

func exitAfter(_ seconds: Double, _ code: Int32) {
    DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { exit(code) }
}

func warn(_ message: String) {
    FileHandle.standardError.write(Data((message + "\n").utf8))
}

final class Delegate: NSObject, UNUserNotificationCenterDelegate {
    let exitsOnClick: Bool

    init(exitsOnClick: Bool) {
        self.exitsOnClick = exitsOnClick
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .list])
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        completionHandler()
        if exitsOnClick {
            finish(EXIT_SUCCESS)
        }
    }
}

func authorize(then: @escaping @Sendable () -> Void) {
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { granted, error in
        if let error {
            finish(error)
        } else if granted {
            then()
        } else {
            finish(notAllowed)
        }
    }
}

func post(_ message: String) {
    let content = UNMutableNotificationContent()
    content.title = "Switchboard"
    content.body = message
    let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
    UNUserNotificationCenter.current().add(request) { error in
        if let error {
            finish(error)
        } else {
            finish(EXIT_SUCCESS)
        }
    }
}

let given = Array(CommandLine.arguments.dropFirst())
// Launch Services may still hand an app it starts a process serial number.
let arguments = given.count == 1 && given[0].hasPrefix("-psn_") ? [] : given
let delegate = Delegate(exitsOnClick: arguments.isEmpty)
UNUserNotificationCenter.current().delegate = delegate
let app = NSApplication.shared
app.setActivationPolicy(.accessory)

switch (arguments.first, arguments.count) {
case ("--post", 2):
    exitAfter(30, EXIT_FAILURE)
    authorize { post(arguments[1]) }
case ("--authorize", 1):
    // Bounded well past the time a user takes to answer macOS's question.
    exitAfter(300, EXIT_FAILURE)
    authorize { finish(EXIT_SUCCESS) }
case (nil, _):
    exitAfter(5, EXIT_SUCCESS)
default:
    warn("usage: Switchboard [--post <message> | --authorize]")
    exit(EXIT_FAILURE)
}

app.run()
