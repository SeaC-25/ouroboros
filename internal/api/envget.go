package api

import "os"

// osGetenv indirection kept tiny: call-time env read per AGENTS.md.
func osGetenv(name string) string { return os.Getenv(name) }
