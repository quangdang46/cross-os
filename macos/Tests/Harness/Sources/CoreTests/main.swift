// The tests.
//
// Run with `swift run CoreTests`. The exit code is the result: 0 for a clean
// run, 1 with the failures listed. `scripts/verify.sh` calls it alongside the
// Go and React suites.
//
// The suite is split across two files because Swift allows top-level
// expressions in exactly one file of a target, and this is it. The other two
// hold the assertions and the stub client.

try await runWireSuite()
try await runClientSuite()
report()
