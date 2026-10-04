import XCTest
@testable import NatKit

final class AppVersionTests: XCTestCase {
    func testAReleasedAppShowsItsVersionAndBuild() {
        let version = AppVersion(infoDictionary: ["CFBundleShortVersionString": "1.4.0", "CFBundleVersion": "212"])
        XCTAssertEqual(version, AppVersion(infoDictionary: ["CFBundleShortVersionString": "1.4.0", "CFBundleVersion": "212"]))
        XCTAssertEqual(version.version, "1.4.0")
        XCTAssertEqual(version.build, "212")
        XCTAssertEqual(version.label, "Version 1.4.0 (212)")
    }

    func testADevExecutableWithNoInfoPlistIsDev() {
        let version = AppVersion(infoDictionary: nil)
        XCTAssertEqual(version.version, "dev")
        XCTAssertEqual(version.build, "dev")
        XCTAssertEqual(version.label, "Version dev")
    }

    func testAnEmptyOrNonStringValueIsUnset() {
        let version = AppVersion(infoDictionary: ["CFBundleShortVersionString": " ", "CFBundleVersion": 7])
        XCTAssertEqual(version.label, "Version dev")
    }

    func testABuildEqualToTheVersionIsNotSaidTwice() {
        let version = AppVersion(infoDictionary: ["CFBundleShortVersionString": "2.0", "CFBundleVersion": "2.0"])
        XCTAssertEqual(version.label, "Version 2.0")
    }
}
