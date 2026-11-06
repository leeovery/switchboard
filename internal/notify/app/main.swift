import AppKit
import UserNotifications

let notAllowed: Int32 = 2

final class Delegate: NSObject, UNUserNotificationCenterDelegate {
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
        exit(EXIT_SUCCESS)
    }
}

func warn(_ message: String) {
    FileHandle.standardError.write(Data((message + "\n").utf8))
}

func exitAfter(_ seconds: Double, _ code: Int32) {
    DispatchQueue.main.asyncAfter(deadline: .now() + seconds) { exit(code) }
}

func authorize(then: @escaping @Sendable () -> Void) {
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { granted, error in
        if let error {
            warn(error.localizedDescription)
        }
        guard granted else { exit(notAllowed) }
        then()
    }
}

func post(_ message: String) {
    let content = UNMutableNotificationContent()
    content.title = "Switchboard"
    content.body = message
    let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
    UNUserNotificationCenter.current().add(request) { error in
        if let error {
            warn(error.localizedDescription)
            exit(EXIT_FAILURE)
        }
        exit(EXIT_SUCCESS)
    }
}

// Launch Services may still hand an app it starts a process serial number.
let arguments = Array(CommandLine.arguments.dropFirst().filter { !$0.hasPrefix("-psn_") })
let delegate = Delegate()
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
    authorize { exit(EXIT_SUCCESS) }
case (nil, _):
    exitAfter(5, EXIT_SUCCESS)
default:
    warn("usage: Switchboard [--post <message> | --authorize]")
    exit(EXIT_FAILURE)
}

app.run()
