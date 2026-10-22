package nativetools

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

// AXML chunk types (Android binary XML).
const (
	axmlChunkStringPool   = 0x0001
	axmlChunkStartElement = 0x0102
)

// AXML value types consumed by the manifest subset.
const (
	axmlTypeString = 0x03
	axmlTypeIntDec = 0x10
)

// axmlStringPool holds the decoded strings of a binary XML string pool chunk.
type axmlStringPool struct {
	strings []string
}

// axmlElement is one decoded start element with its attributes.
type axmlElement struct {
	Name       string
	Attributes map[string]string
}

// axmlAttribute is a raw attribute before value rendering.
type axmlAttribute struct {
	Name     string
	RawValue string
	DataType uint8
	Data     uint32
}

// parseAXMLManifest decodes the subset of binary Android XML (AXML) needed to
// read AndroidManifest.xml: the string pool, start-element chunks and
// attribute values of string or integer type. Unknown chunks are skipped by
// chunk size; malformed headers are errors.
func parseAXMLManifest(data []byte) ([]axmlElement, error) {
	if len(data) < 8 {
		return nil, errors.New("axml: data too short")
	}
	if binary.LittleEndian.Uint32(data[0:4]) != 0x00080003 {
		return nil, fmt.Errorf("axml: bad magic %08x", binary.LittleEndian.Uint32(data[0:4]))
	}
	declaredSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if declaredSize > len(data) {
		return nil, fmt.Errorf("axml: declared size %d exceeds data %d", declaredSize, len(data))
	}

	pool := &axmlStringPool{}
	elements := []axmlElement{}
	offset := 8
	for offset+8 <= len(data) {
		chunkType := binary.LittleEndian.Uint16(data[offset : offset+2])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if chunkSize < 8 || offset+chunkSize > len(data) {
			return nil, fmt.Errorf("axml: bad chunk size %d at offset %d", chunkSize, offset)
		}
		body := data[offset+8 : offset+chunkSize]
		switch chunkType {
		case axmlChunkStringPool:
			if err := pool.decode(body); err != nil {
				return nil, err
			}
		case axmlChunkStartElement:
			element, err := pool.decodeStartElement(body)
			if err != nil {
				return nil, err
			}
			elements = append(elements, element)
		}
		offset += chunkSize
	}
	return elements, nil
}

// decode parses a string pool chunk body (the 20 header bytes that follow the
// 8-byte chunk header, then the offset table and string data).
func (p *axmlStringPool) decode(body []byte) error {
	if len(body) < 20 {
		return errors.New("axml: string pool header too short")
	}
	stringCount := int(binary.LittleEndian.Uint32(body[0:4]))
	flags := binary.LittleEndian.Uint32(body[4:8])
	stringsStart := int(binary.LittleEndian.Uint32(body[12:16]))
	isUTF8 := flags&0x100 != 0

	if stringCount > 0 && len(body) < 20+4*stringCount {
		return errors.New("axml: string pool offsets truncated")
	}
	offsets := make([]int, stringCount)
	for i := range offsets {
		offsets[i] = int(binary.LittleEndian.Uint32(body[20+4*i : 24+4*i]))
	}

	strings := make([]string, 0, stringCount)
	for _, rel := range offsets {
		// String data begins at chunk start + stringsStart; body starts at
		// chunk start + 8, so the in-body base is stringsStart - 8 + rel.
		base := stringsStart - 8 + rel
		if base < 0 || base >= len(body) {
			strings = append(strings, "")
			continue
		}
		if isUTF8 {
			strings = append(strings, decodeAXMLUTF8(body[base:]))
		} else {
			strings = append(strings, decodeAXMLUTF16(body[base:]))
		}
	}
	p.strings = strings
	return nil
}

// decodeAXMLUTF16Length reads one AXML length prefix (1 or 2 uint16 units).
// A unit below 0x8000 is the whole length; otherwise the low 15 bits of the
// first unit are the high part and one more unit carries the low part.
func decodeAXMLUTF16Length(data []byte) (length, size int) {
	if len(data) < 2 {
		return 0, 0
	}
	first := binary.LittleEndian.Uint16(data[0:2])
	if first&0x8000 == 0 {
		return int(first), 2
	}
	if len(data) < 4 {
		return 0, 0
	}
	second := binary.LittleEndian.Uint16(data[2:4])
	return int(first&0x7fff)<<16 | int(second), 4
}

// decodeAXMLUTF16 reads one length-prefixed UTF-16LE string.
func decodeAXMLUTF16(data []byte) string {
	length, size := decodeAXMLUTF16Length(data)
	if length <= 0 || size+2*length > len(data) {
		return ""
	}
	units := make([]uint16, length)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[size+2*i : size+2*i+2])
	}
	return string(utf16.Decode(units))
}

// decodeAXMLUTF8 reads one length-prefixed UTF-8 string: a UTF-16 length
// prefix (skipped), then the byte length, then the content.
func decodeAXMLUTF8(data []byte) string {
	pos := 0
	// UTF-16 length prefix: 1-2 bytes, high-bit continuation.
	for pos < len(data) && data[pos]&0x80 != 0 {
		pos++
	}
	if pos >= len(data) {
		return ""
	}
	pos++ // final UTF-16 length byte
	if pos >= len(data) {
		return ""
	}
	byteLen := int(data[pos])
	pos++
	if pos+byteLen > len(data) {
		return ""
	}
	return string(data[pos : pos+byteLen])
}

// decodeStartElement parses a start element body after the chunk header:
// line(4) comment(4) ns(4) name(4) attribute_start(2) attribute_size(2)
// attribute_count(2) id_index(2) class_index(2) style_index(2), then the
// attribute records.
func (p *axmlStringPool) decodeStartElement(body []byte) (axmlElement, error) {
	// body layout after the 8-byte chunk header: line(4) comment(4) ns(4)
	// name(4) attributeStart(2) attributeSize(2) attributeCount(2)
	// idIndex(2) classIndex(2) styleIndex(2) — 28 bytes — then attributes.
	if len(body) < 28 {
		return axmlElement{}, errors.New("axml: start element too short")
	}
	name := p.stringAt(int(binary.LittleEndian.Uint32(body[12:16])))
	attrStart := int(binary.LittleEndian.Uint16(body[16:18]))
	attrSize := int(binary.LittleEndian.Uint16(body[18:20]))
	attrCount := int(binary.LittleEndian.Uint16(body[20:22]))
	if attrSize == 0 {
		attrSize = 20
	}

	element := axmlElement{Name: name, Attributes: map[string]string{}}
	for i := range attrCount {
		// attributeStart is relative to the attrExt struct, which begins at
		// body[8] (after line+comment), so the record base is body[8+attrStart].
		base := 8 + attrStart + i*attrSize
		if base+20 > len(body) {
			break
		}
		attr := p.attribute(body[base : base+20])
		if attr.Name == "" {
			continue
		}
		element.Attributes[attr.Name] = attr.value()
	}
	return element, nil
}

// attribute decodes one attribute record: ns(4) name(4) raw_value(4) then the
// typed value: size(2) res0(1) dataType(1) data(4).
func (p *axmlStringPool) attribute(record []byte) axmlAttribute {
	attr := axmlAttribute{
		Name: p.stringAt(int(binary.LittleEndian.Uint32(record[4:8]))),
	}
	rawIndex := int(int32(binary.LittleEndian.Uint32(record[8:12])))
	if rawIndex >= 0 && rawIndex < len(p.strings) {
		attr.RawValue = p.strings[rawIndex]
	}
	attr.DataType = record[15]
	attr.Data = binary.LittleEndian.Uint32(record[16:20])
	return attr
}

// value renders the attribute value as a string.
func (a axmlAttribute) value() string {
	if a.RawValue != "" {
		return a.RawValue
	}
	if a.DataType == axmlTypeIntDec {
		return fmt.Sprintf("%d", a.Data)
	}
	return ""
}

// stringAt returns the pool string at index, or "" when out of range.
func (p *axmlStringPool) stringAt(index int) string {
	if index < 0 || index >= len(p.strings) {
		return ""
	}
	return p.strings[index]
}
