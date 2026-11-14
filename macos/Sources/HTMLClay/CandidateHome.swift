import Foundation
import Darwin

// Until milestone 7 the candidate always runs on a throwaway home, so it cannot
// change the installed app's config, login item or file types.

struct CandidateHomeFailure: Error, CustomStringConvertible {
    var description: String
}

enum CandidateHome {
    /// HTMLCLAY_CANDIDATE_HOME when set and absolute, else a new /tmp/hc-<8 hex>. Resolved with realpath.
    static func resolve() throws -> URL {
        let manager = FileManager.default
        if let given = ProcessInfo.processInfo.environment["HTMLCLAY_CANDIDATE_HOME"],
           given.hasPrefix("/") {
            try manager.createDirectory(atPath: given, withIntermediateDirectories: true)
            return try real(URL(fileURLWithPath: given))
        }
        let path = "/tmp/hc-" + hexadecimal(4)
        try manager.createDirectory(atPath: path, withIntermediateDirectories: true)
        return try real(URL(fileURLWithPath: path))
    }

    /// Writes <home>/c/htmlclay/config.json = {"startOnLogin": false, "port": <free loopback port>, "updates": "off"} unless it exists.
    static func prepare(_ home: URL) throws {
        let directory = home.appendingPathComponent("c/htmlclay", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let file = directory.appendingPathComponent("config.json")
        guard !FileManager.default.fileExists(atPath: file.path) else { return }
        let body = "{\"startOnLogin\": false, \"port\": \(try freeLoopbackPort()), \"updates\": \"off\"}\n"
        try body.write(to: file, atomically: true, encoding: .utf8)
    }

    static func environment(_ home: URL) -> [String: String] {
        ["HOME": home.path, "HTMLCLAY_CONFIG_BASE": home.appendingPathComponent("c", isDirectory: true).path]
    }

    private static func real(_ url: URL) throws -> URL {
        var buffer = [CChar](repeating: 0, count: Int(PATH_MAX))
        guard realpath(url.path, &buffer) != nil else {
            throw CandidateHomeFailure(description: "realpath \(url.path): \(errno)")
        }
        return URL(fileURLWithPath: String(cString: buffer))
    }

    private static func hexadecimal(_ bytes: Int) -> String {
        var generator = SystemRandomNumberGenerator()
        return (0 ..< bytes).map { _ in String(format: "%02x", UInt8.random(in: 0 ... 255, using: &generator)) }.joined()
    }

    private static func freeLoopbackPort() throws -> Int {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else { throw CandidateHomeFailure(description: "socket: \(errno)") }
        defer { close(fd) }
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else { throw CandidateHomeFailure(description: "bind: \(errno)") }
        var actual = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let named = withUnsafeMutablePointer(to: &actual) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                getsockname(fd, $0, &length)
            }
        }
        guard named == 0 else { throw CandidateHomeFailure(description: "getsockname: \(errno)") }
        return Int(UInt16(bigEndian: actual.sin_port))
    }
}
