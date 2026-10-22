// Command fakenode is a test stand-in for the node executable used by the
// electronruntime package tests. It validates the driver path argument and
// emulates node exit behavior controlled by environment variables:
//
//	FAKENODE_STDOUT   text written to stdout (default: "electron runtime contract: ok")
//	FAKENODE_STDERR   text written to stderr
//	FAKENODE_EXIT     exit code (default 0)
//	FAKENODE_MUSTCONTAIN  substring the driver file must contain, else exit 42
package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing driver path")
		os.Exit(64)
	}
	driver := os.Args[1]
	content, err := os.ReadFile(driver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read driver %s: %v\n", driver, err)
		os.Exit(65)
	}
	if marker := os.Getenv("FAKENODE_MUSTCONTAIN"); marker != "" && !contains(string(content), marker) {
		fmt.Fprintf(os.Stderr, "driver %s missing marker %q\n", driver, marker)
		os.Exit(42)
	}
	if out := os.Getenv("FAKENODE_STDOUT"); out != "" {
		fmt.Println(out)
	} else {
		fmt.Println("electron runtime contract: ok")
	}
	if e := os.Getenv("FAKENODE_STDERR"); e != "" {
		fmt.Fprintln(os.Stderr, e)
	}
	code, _ := strconv.Atoi(os.Getenv("FAKENODE_EXIT"))
	os.Exit(code)
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && index(haystack, needle) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
