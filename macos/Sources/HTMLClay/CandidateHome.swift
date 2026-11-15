import Foundation
import PreviewCore
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

    /// Writes <config base>/htmlclay/config.json = {"startOnLogin": false, "port": <free loopback port>, "updates": "off"} unless it exists.
    static func prepare(_ home: URL) throws {
        let directory = configBase(home).appendingPathComponent("htmlclay", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let file = directory.appendingPathComponent("config.json")
        guard !FileManager.default.fileExists(atPath: file.path) else { return }
        let body = "{\"startOnLogin\": false, \"port\": \(try freeLoopbackPort()), \"updates\": \"off\"}\n"
        try body.write(to: file, atomically: true, encoding: .utf8)
    }

    /// The persona --profile names, or nil when the app was launched without one.
    static func profile(_ arguments: [String]) throws -> Persona? {
        guard let index = arguments.firstIndex(of: "--profile") else { return nil }
        guard index + 1 < arguments.count else {
            throw CandidateHomeFailure(description: "--profile needs a persona name")
        }
        let name = arguments[index + 1]
        guard let persona = Persona(rawValue: name) else {
            throw CandidateHomeFailure(description: "--profile \(name) is not a persona")
        }
        guard persona.isReal else {
            throw CandidateHomeFailure(description: "--profile \(name) is sample only until milestone \(persona.milestone ?? 0)")
        }
        return persona
    }

    /// Throws unless a persona can seed this home: nothing trusts a folder here yet, and a
    /// home given by HTMLCLAY_CANDIDATE_HOME holds nothing prepare did not write.
    static func requireFresh(_ home: URL) throws {
        for key in ["workspaceFolders", "trustedFolders"] {
            guard let folders = trustedFolders(home, key) else { continue }
            guard folders.isEmpty else {
                throw CandidateHomeFailure(description: "\(home.path) already trusts \(folders.count) folder(s)")
            }
        }
        guard let given = ProcessInfo.processInfo.environment["HTMLCLAY_CANDIDATE_HOME"], given.hasPrefix("/") else { return }
        let allowed: Set<String> = ["c", "c/htmlclay", "c/htmlclay/config.json"]
        for path in try relativePaths(under: home) where !allowed.contains(path) {
            throw CandidateHomeFailure(description: "\(home.path) already holds \(path), so a profile has nothing fresh to seed")
        }
    }

    private static func trustedFolders(_ home: URL, _ key: String) -> [Any]? {
        let file = home.appendingPathComponent("c/htmlclay/config.json")
        guard let data = try? Data(contentsOf: file),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return nil }
        return json[key] as? [Any]
    }

    private static func relativePaths(under home: URL) throws -> [String] {
        var found: [String] = []
        func walk(_ directory: URL, _ prefix: String) throws {
            for name in try FileManager.default.contentsOfDirectory(atPath: directory.path).sorted() {
                let relative = prefix.isEmpty ? name : prefix + "/" + name
                found.append(relative)
                var isDirectory: ObjCBool = false
                let child = directory.appendingPathComponent(name)
                if FileManager.default.fileExists(atPath: child.path, isDirectory: &isDirectory), isDirectory.boolValue {
                    try walk(child, relative)
                }
            }
        }
        try walk(home, "")
        return found
    }

    static func environment(_ home: URL) -> [String: String] {
        ["HOME": home.path, "HTMLCLAY_CONFIG_BASE": configBase(home).path]
    }

    /// HTMLCLAY_CONFIG_BASE when set and absolute, so an update rehearsal can put the host on the config folder the old app uses; else <home>/c.
    static func configBase(_ home: URL) -> URL {
        if let given = ProcessInfo.processInfo.environment["HTMLCLAY_CONFIG_BASE"], given.hasPrefix("/") {
            return URL(fileURLWithPath: given, isDirectory: true)
        }
        return home.appendingPathComponent("c", isDirectory: true)
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
