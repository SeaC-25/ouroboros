package nativetools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildAXMLStringPool builds a UTF-16 string pool chunk for the given strings.
func buildAXMLStringPool(strs []string) []byte {
	stringData := bytes.Buffer{}
	offsets := make([]int, len(strs))
	for i, s := range strs {
		offsets[i] = stringData.Len()
		runes := []rune(s)
		binary.Write(&stringData, binary.LittleEndian, uint16(len(runes)))
		for _, r := range runes {
			binary.Write(&stringData, binary.LittleEndian, uint16(r))
		}
		binary.Write(&stringData, binary.LittleEndian, uint16(0))
	}

	header := bytes.Buffer{}
	binary.Write(&header, binary.LittleEndian, uint16(0x0001)) // type
	binary.Write(&header, binary.LittleEndian, uint16(28))     // header size
	chunkSize := 28 + 4*len(strs) + stringData.Len()
	binary.Write(&header, binary.LittleEndian, uint32(chunkSize))
	binary.Write(&header, binary.LittleEndian, uint32(len(strs)))      // string count
	binary.Write(&header, binary.LittleEndian, uint32(0))              // flags: UTF-16
	binary.Write(&header, binary.LittleEndian, uint32(0))              // style count
	binary.Write(&header, binary.LittleEndian, uint32(28+4*len(strs))) // strings start
	binary.Write(&header, binary.LittleEndian, uint32(0))              // styles start

	pool := header.Bytes()
	for _, off := range offsets {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(off))
		pool = append(pool, b[:]...)
	}
	pool = append(pool, stringData.Bytes()...)
	return pool
}

// buildAXMLStartElement builds one start-element chunk with string-pool
// attribute records (name/value as pool indices, string type).
func buildAXMLStartElement(nameIndex int, attrs [][2]int) []byte {
	attrCount := len(attrs)
	bodySize := 28 + 20*attrCount
	chunk := bytes.Buffer{}
	binary.Write(&chunk, binary.LittleEndian, uint16(0x0102)) // type
	binary.Write(&chunk, binary.LittleEndian, uint16(16))     // header size
	binary.Write(&chunk, binary.LittleEndian, uint32(8+bodySize))
	binary.Write(&chunk, binary.LittleEndian, uint32(1))          // line
	binary.Write(&chunk, binary.LittleEndian, uint32(0xFFFFFFFF)) // comment
	binary.Write(&chunk, binary.LittleEndian, uint32(0xFFFFFFFF)) // ns
	binary.Write(&chunk, binary.LittleEndian, uint32(nameIndex))
	binary.Write(&chunk, binary.LittleEndian, uint16(20)) // attributeStart (0x14)
	binary.Write(&chunk, binary.LittleEndian, uint16(20)) // attributeSize
	binary.Write(&chunk, binary.LittleEndian, uint16(attrCount))
	binary.Write(&chunk, binary.LittleEndian, uint16(0)) // idIndex
	binary.Write(&chunk, binary.LittleEndian, uint16(0)) // classIndex
	binary.Write(&chunk, binary.LittleEndian, uint16(0)) // styleIndex
	for _, attr := range attrs {
		binary.Write(&chunk, binary.LittleEndian, uint32(0xFFFFFFFF)) // ns
		binary.Write(&chunk, binary.LittleEndian, uint32(attr[0]))    // name index
		binary.Write(&chunk, binary.LittleEndian, uint32(attr[1]))    // raw value index
		binary.Write(&chunk, binary.LittleEndian, uint16(8))          // typed value size
		binary.Write(&chunk, binary.LittleEndian, uint8(0))           // res0
		binary.Write(&chunk, binary.LittleEndian, uint8(0x03))        // dataType: STRING
		binary.Write(&chunk, binary.LittleEndian, uint32(attr[1]))    // data: pool index
	}
	return chunk.Bytes()
}

// buildTestAPK writes a minimal APK with a real binary AXML manifest and a
// stub dex header carrying a known method_ids_size.
func buildTestAPK(t *testing.T) string {
	t.Helper()
	// String pool: 0=manifest 1=package 2=com.example.app 3=versionName
	// 4=1.0 5=versionCode 6=7 7=uses-sdk 8=minSdkVersion 9=21
	// 10=targetSdkVersion 11=30 12=application 13=activity 14=name
	// 15=.MainActivity 16=exported 17=true 18=uses-permission
	// 19=android.permission.INTERNET 20=android
	strs := []string{
		"manifest", "package", "com.example.app", "versionName", "1.0",
		"versionCode", "7", "uses-sdk", "minSdkVersion", "21",
		"targetSdkVersion", "30", "application", "activity", "name",
		".MainActivity", "exported", "true", "uses-permission",
		"android.permission.INTERNET", "android",
	}

	manifestBody := bytes.Buffer{}
	// File header: magic + size (patched later).
	binary.Write(&manifestBody, binary.LittleEndian, uint32(0x00080003))
	binary.Write(&manifestBody, binary.LittleEndian, uint32(0)) // size placeholder
	manifestBody.Write(buildAXMLStringPool(strs))
	manifestBody.Write(buildAXMLStartElement(0, [][2]int{{1, 2}, {3, 4}, {5, 6}})) // manifest
	manifestBody.Write(buildAXMLStartElement(7, [][2]int{{8, 9}, {10, 11}}))       // uses-sdk
	manifestBody.Write(buildAXMLStartElement(12, nil))                             // application
	manifestBody.Write(buildAXMLStartElement(13, [][2]int{{14, 15}, {16, 17}}))    // activity exported
	manifestBody.Write(buildAXMLStartElement(18, [][2]int{{14, 19}}))              // uses-permission
	manifestData := manifestBody.Bytes()
	binary.LittleEndian.PutUint32(manifestData[4:8], uint32(len(manifestData)))

	// Stub dex: 0x70-byte header with method_ids_size=42 at 0x58.
	dex := make([]byte, 0x70)
	copy(dex, "dex\n035\x00")
	binary.LittleEndian.PutUint32(dex[0x58:0x5C], 42)

	apkPath := filepath.Join(t.TempDir(), "test.apk")
	out, err := os.Create(apkPath)
	if err != nil {
		t.Fatalf("create apk: %v", err)
	}
	defer func() { _ = out.Close() }()
	writer := zip.NewWriter(out)
	manifestEntry, err := writer.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := manifestEntry.Write(manifestData); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dexEntry, err := writer.Create("classes.dex")
	if err != nil {
		t.Fatalf("create dex entry: %v", err)
	}
	if _, err := dexEntry.Write(dex); err != nil {
		t.Fatalf("write dex: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return apkPath
}

func TestDecompileAPKApktoolExtractsManifestSummary(t *testing.T) {
	apkPath := buildTestAPK(t)

	out, err := DecompileAPKApktool(context.Background(), apkPath, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DecompileAPKApktool: %v", err)
	}
	for _, want := range []string{
		"package=com.example.app",
		"versionName=1.0",
		"versionCode=7",
		"sdk min=21 target=30",
		"uses-permission android.permission.INTERNET",
		"activity .MainActivity",
		"exported .MainActivity",
		"dex files=1 methods=42",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in apktool output:\n%s", want, out)
		}
	}
}

func TestDecompileAPKApktoolRejectsMissingManifest(t *testing.T) {
	apkPath := filepath.Join(t.TempDir(), "empty.apk")
	out, err := os.Create(apkPath)
	if err != nil {
		t.Fatalf("create apk: %v", err)
	}
	writer := zip.NewWriter(out)
	if _, err := writer.Create("classes.dex"); err != nil {
		t.Fatalf("create dex entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	_ = out.Close()

	if _, err := DecompileAPKApktool(context.Background(), apkPath, Options{Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected an error for an APK without a manifest")
	}
}

func TestScanStaticCodeMobsfEmitsMobsfscanJSON(t *testing.T) {
	dir := t.TempDir()
	vulnerable := filepath.Join(dir, "MainActivity.java")
	source := "package com.example;\n" +
		"public class MainActivity {\n" +
		"    private static final String KEY = \"AKIAIOSFODNN7EXAMPLE\";\n" +
		"    void init() { webView.getSettings().setJavaScriptEnabled(true); }\n" +
		"}\n"
	if err := os.WriteFile(vulnerable, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	clean := filepath.Join(dir, "Clean.java")
	if err := os.WriteFile(clean, []byte("package com.example;\npublic class Clean {}\n"), 0o644); err != nil {
		t.Fatalf("write clean: %v", err)
	}

	out, err := ScanStaticCodeMobsf(context.Background(), dir, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanStaticCodeMobsf: %v", err)
	}
	var decoded mobsfscanOutput
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode mobsfscan output: %v (%s)", err, out)
	}
	if _, ok := decoded.Results["hardcoded_api_key"]; !ok {
		t.Fatalf("expected hardcoded_api_key finding, got: %s", out)
	}
	if _, ok := decoded.Results["webview_js_enabled"]; !ok {
		t.Fatalf("expected webview_js_enabled finding, got: %s", out)
	}
	result := decoded.Results["hardcoded_api_key"]
	if len(result.Files) == 0 || result.Files[0].StartLine != 3 {
		t.Fatalf("expected the api key on line 3, got %#v", result.Files)
	}
	if result.Metadata.Severity != "HIGH" || result.Metadata.CWE != "CWE-798" {
		t.Fatalf("unexpected metadata: %#v", result.Metadata)
	}
}
