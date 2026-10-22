package nativetools

// Operation IDs for the artifact-analysis tools. They mirror the
// managed_binary operation names in internal/toolchain/authored_registry.go
// and are shared by every handler file for those operations.
const (
	// OpStaticCodeScan is the semgrep/mobsfscan-equivalent operation.
	OpStaticCodeScan = "static_code_scan"
	// OpApkDecompile is the apktool/jadx-equivalent operation.
	OpApkDecompile = "apk_decompile"
)
