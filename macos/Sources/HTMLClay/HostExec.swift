import Darwin
import Foundation

enum HostExec {
    static func replace(_ arguments: [String]) -> Never {
        let host = hostPath()
        var argv: [UnsafeMutablePointer<CChar>?] = arguments.map { strdup($0) }
        argv.append(nil)
        execv(host, &argv)
        let reason = String(cString: strerror(errno))
        FileHandle.standardError.write(Data("htmlclay: cannot run \(host): \(reason)\n".utf8))
        exit(127)
    }

    static func hostPath() -> String {
        var size: UInt32 = 0
        _ = _NSGetExecutablePath(nil, &size)
        var buffer = [CChar](repeating: 0, count: Int(size) + 1)
        guard _NSGetExecutablePath(&buffer, &size) == 0 else { return "htmlclay-host" }
        var resolved = [CChar](repeating: 0, count: Int(PATH_MAX))
        let program = realpath(buffer, &resolved) != nil ? String(cString: resolved) : String(cString: buffer)
        return ((program as NSString).deletingLastPathComponent as NSString).appendingPathComponent("htmlclay-host")
    }
}
