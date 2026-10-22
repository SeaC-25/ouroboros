package nativetools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// OpMetadataExtract is the exiftool-equivalent operation.
const OpMetadataExtract = "metadata_extract"

// MetadataFile is one exiftool-json style record: SourceFile plus tag/value
// pairs. Keys keep exiftool naming so existing fixtures stay comparable.
type MetadataFile struct {
	SourceFile string
	Tags       map[string]any
}

// MarshalJSON emits {"SourceFile": path, "<tag>": value, ...} like exiftool -json.
func (m MetadataFile) MarshalJSON() ([]byte, error) {
	object := make(map[string]any, len(m.Tags)+1)
	for key, value := range m.Tags {
		object[key] = value
	}
	object["SourceFile"] = m.SourceFile
	return json.Marshal(object)
}

// ExtractMetadata inspects each path and returns exiftool-compatible JSON.
// Unsupported formats degrade to File:* tags instead of failing, mirroring
// exiftool's tolerance of unknown containers.
func ExtractMetadata(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("metadata_extract requires a file or directory path")
	}
	paths, err := collectMetadataPaths(target)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("metadata_extract found no regular files under %s", target)
	}

	records := make([]MetadataFile, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		record, err := metadataForFile(path)
		if err != nil {
			// exiftool still reports a record with File:* fallbacks.
			record = fallbackMetadata(path)
		}
		records = append(records, record)
	}

	encoded, err := json.Marshal(records)
	if err != nil {
		return "", fmt.Errorf("encode metadata records: %w", err)
	}
	return string(encoded), nil
}

func collectMetadataPaths(target string) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat metadata target: %w", err)
	}
	if !info.IsDir() {
		return []string{target}, nil
	}
	paths := []string{}
	err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk metadata target: %w", err)
	}
	sort.Strings(paths)
	if len(paths) > 256 {
		paths = paths[:256]
	}
	return paths, nil
}

func fallbackMetadata(path string) MetadataFile {
	record := MetadataFile{SourceFile: path, Tags: map[string]any{}}
	if info, err := os.Stat(path); err == nil {
		record.Tags["File:FileSize"] = info.Size()
		record.Tags["File:FileModifyDate"] = info.ModTime().UTC().Format(time.RFC3339)
		record.Tags["File:FileName"] = filepath.Base(path)
	}
	return record
}

func metadataForFile(path string) (MetadataFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MetadataFile{}, err
	}
	record := fallbackMetadata(path)
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		readJPEGMetadata(data, &record)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		readPNGMetadata(data, &record)
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		readGIFMetadata(data, &record)
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		readZIPMetadata(path, &record)
	case bytes.HasPrefix(data, []byte("%PDF-")):
		readPDFMetadata(data, &record)
	default:
		record.Tags["File:FileType"] = "unknown"
	}
	return record, nil
}

// jpegExifTags maps EXIF tag ids to exiftool-style tag names for the tags we
// decode. Values are exposed without the "EXIF:" prefix only where exiftool
// would emit them bare.
var jpegExifTags = map[uint16]string{
	0x010F: "Make",
	0x0110: "Model",
	0x0112: "Orientation",
	0x011A: "XResolution",
	0x011B: "YResolution",
	0x0132: "ModifyDate",
	0x9003: "DateTimeOriginal",
	0x9004: "CreateDate",
	0xA002: "ExifImageWidth",
	0xA003: "ExifImageHeight",
	0x829A: "ExposureTime",
	0x829D: "FNumber",
	0x8827: "ISO",
}

func readJPEGMetadata(data []byte, record *MetadataFile) {
	record.Tags["File:FileType"] = "JPEG"
	width, height, ok := jpegSOFDimensions(data)
	if ok {
		record.Tags["ImageWidth"] = width
		record.Tags["ImageHeight"] = height
	}
	record.Tags["EXIF:ExifByteOrder"] = "Big-endian (Motorola, MM)"

	for offset := 2; offset+4 <= len(data); {
		if data[offset] != 0xFF {
			offset++
			continue
		}
		marker := data[offset+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			offset += 2
			continue
		}
		if offset+4 > len(data) {
			return
		}
		size := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if size < 2 || offset+2+size > len(data) {
			return
		}
		if marker == 0xE1 {
			readExifAPP1(data[offset+4:offset+2+size], record)
		}
		if marker == 0xDA { // start of scan: no more metadata segments
			return
		}
		offset += 2 + size
	}
}

func readExifAPP1(segment []byte, record *MetadataFile) {
	const header = "Exif\x00\x00"
	if !bytes.HasPrefix(segment, []byte(header)) {
		return
	}
	tiff := segment[len(header):]
	if len(tiff) < 8 {
		return
	}
	var order binary.ByteOrder = binary.BigEndian
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	if order.Uint16(tiff[2:4]) != 0x2A {
		return
	}
	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset <= 0 || ifdOffset+2 > len(tiff) {
		return
	}
	readIFD(tiff, ifdOffset, order, record)
}

func readIFD(tiff []byte, offset int, order binary.ByteOrder, record *MetadataFile) {
	if offset+2 > len(tiff) {
		return
	}
	count := int(order.Uint16(tiff[offset : offset+2]))
	entry := offset + 2
	for i := 0; i < count; i++ {
		if entry+12 > len(tiff) {
			return
		}
		tag := order.Uint16(tiff[entry : entry+2])
		format := order.Uint16(tiff[entry+2 : entry+4])
		components := int(order.Uint32(tiff[entry+4 : entry+8]))
		valueBytes := componentBytes(format, components)
		if name, ok := jpegExifTags[tag]; ok {
			if value, ok := exifValue(tiff, entry+8, valueBytes, format, order, components); ok {
				record.Tags[name] = value
			}
		}
		// ExifIFD pointer (0x8769) — follow one level for date/exposure tags.
		if tag == 0x8769 && valueBytes <= 4 {
			sub := int(order.Uint32(tiff[entry+8 : entry+12]))
			if sub > 0 && sub < len(tiff) {
				readIFD(tiff, sub, order, record)
			}
		}
		entry += 12
	}
}

func componentBytes(format uint16, components int) int {
	size := 0
	switch format {
	case 1, 2, 6, 7:
		size = 1
	case 3, 8:
		size = 2
	case 4, 9, 11:
		size = 4
	case 5, 10, 12:
		size = 8
	default:
		return 0
	}
	return size * components
}

func exifValue(tiff []byte, valueOffset int, size int, format uint16, order binary.ByteOrder, components int) (any, bool) {
	var raw []byte
	if size <= 4 {
		raw = tiff[valueOffset : valueOffset+4]
	} else {
		offset := int(order.Uint32(tiff[valueOffset : valueOffset+4]))
		if offset < 0 || offset+size > len(tiff) {
			return nil, false
		}
		raw = tiff[offset : offset+size]
	}
	switch format {
	case 2: // ASCII
		return strings.TrimRight(string(raw), "\x00"), true
	case 3: // SHORT
		if len(raw) < 2 {
			return nil, false
		}
		return int(order.Uint16(raw[:2])), true
	case 4, 9: // LONG/SLONG
		if len(raw) < 4 {
			return nil, false
		}
		return int(order.Uint32(raw[:4])), true
	case 5, 10: // RATIONAL/SRATIONAL
		if len(raw) < 8 {
			return nil, false
		}
		num := float64(order.Uint32(raw[:4]))
		den := float64(order.Uint32(raw[4:8]))
		if den == 0 {
			return nil, false
		}
		return num / den, true
	default:
		_ = components
		return nil, false
	}
}

// jpegSOFDimensions scans for a start-of-frame marker to read image size.
func jpegSOFDimensions(data []byte) (int, int, bool) {
	for offset := 2; offset+10 <= len(data); {
		if data[offset] != 0xFF {
			offset++
			continue
		}
		marker := data[offset+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			offset += 2
			continue
		}
		if marker == 0xDA {
			return 0, 0, false
		}
		size := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		if size < 2 {
			return 0, 0, false
		}
		isSOF := marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
		if isSOF && offset+9 <= len(data) {
			height := int(binary.BigEndian.Uint16(data[offset+5 : offset+7]))
			width := int(binary.BigEndian.Uint16(data[offset+7 : offset+9]))
			return width, height, true
		}
		offset += 2 + size
	}
	return 0, 0, false
}

func readPNGMetadata(data []byte, record *MetadataFile) {
	record.Tags["File:FileType"] = "PNG"
	if len(data) >= 33 && string(data[12:16]) == "IHDR" {
		record.Tags["ImageWidth"] = int(binary.BigEndian.Uint32(data[16:20]))
		record.Tags["ImageHeight"] = int(binary.BigEndian.Uint32(data[20:24]))
		record.Tags["BitDepth"] = int(data[24])
		record.Tags["ColorType"] = int(data[25])
	}
	for offset := 8; offset+12 <= len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		chunkType := string(data[offset+4 : offset+8])
		if length < 0 || offset+12+length > len(data) {
			return
		}
		payload := data[offset+8 : offset+8+length]
		switch chunkType {
		case "tEXt":
			if keyword, text, ok := strings.Cut(string(payload), "\x00"); ok {
				record.Tags["PNG:"+keyword] = text
			}
		case "iTXt":
			if keyword, rest, ok := strings.Cut(string(payload), "\x00"); ok {
				record.Tags["PNG:"+keyword] = strings.TrimSpace(rest)
			}
		case "IEND":
			return
		}
		offset += 12 + length
	}
}

func readGIFMetadata(data []byte, record *MetadataFile) {
	record.Tags["File:FileType"] = "GIF"
	if len(data) >= 10 {
		record.Tags["GIFVersion"] = string(data[3:6])
		record.Tags["ImageWidth"] = int(binary.LittleEndian.Uint16(data[6:8]))
		record.Tags["ImageHeight"] = int(binary.LittleEndian.Uint16(data[8:10]))
	}
}

func readZIPMetadata(path string, record *MetadataFile) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		record.Tags["File:FileType"] = "ZIP"
		return
	}
	defer func() { _ = reader.Close() }()
	record.Tags["File:FileType"] = "ZIP"
	record.Tags["ZIP:ZipEntryCount"] = len(reader.File)
	total := int64(0)
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		total += int64(file.UncompressedSize64)
		names = append(names, file.Name)
		if len(names) >= 20 {
			break
		}
	}
	record.Tags["ZIP:UncompressedSize"] = total
	record.Tags["ZIP:Entries"] = names
}

func readPDFMetadata(data []byte, record *MetadataFile) {
	record.Tags["File:FileType"] = "PDF"
	record.Tags["PDF:PDFVersion"] = strings.TrimSpace(strings.TrimPrefix(string(data[:min(len(data), 8)]), "%PDF-"))
	for _, key := range []string{"Title", "Author", "Producer", "Creator"} {
		if value, ok := pdfInfoValue(data, key); ok {
			record.Tags["PDF:"+key] = value
		}
	}
}

// pdfInfoValue extracts /Key (value) from the trailer Info dictionary.
func pdfInfoValue(data []byte, key string) (string, bool) {
	needle := "/" + key
	idx := bytes.Index(data, []byte(needle))
	if idx < 0 {
		return "", false
	}
	rest := data[idx+len(needle):]
	for _, delimiter := range []byte{'(', '<'} {
		if open := bytes.IndexByte(rest, delimiter); open >= 0 && open < 16 {
			closeDelimiter := byte(')')
			if delimiter == '<' {
				closeDelimiter = '>'
			}
			if close := bytes.IndexByte(rest[open+1:], closeDelimiter); close >= 0 {
				return strings.TrimSpace(string(rest[open+1 : open+1+close])), true
			}
		}
	}
	return "", false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
