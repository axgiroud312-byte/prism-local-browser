package backup

import (
	"context"
	"encoding/binary"
	"io"
)

// archive/zip allocates central-directory objects before returning. Bound the
// actual directory first (not only its untrusted EOCD count), including ZIP64.
func checkZIPBudget(ctx context.Context, file io.ReaderAt, size int64) error {
	if size < 22 {
		return invalid("not-native-zip")
	}
	n := int64(65557)
	if size < n {
		n = size
	}
	tail := make([]byte, n)
	if _, err := file.ReadAt(tail, size-n); err != nil {
		return err
	}
	// Native v1 exports no ZIP comment or trailing bytes. Pin the same final
	// EOCD archive/zip will use, rather than letting two parsers select different
	// embedded records from a crafted comment/trailing-data polyglot.
	end := len(tail) - 22
	if binary.LittleEndian.Uint32(tail[end:end+4]) != 0x06054b50 || binary.LittleEndian.Uint16(tail[end+20:end+22]) != 0 {
		return invalid("zip-end-record")
	}
	e := tail[end:]
	boundary := size - n + int64(end)
	if binary.LittleEndian.Uint16(e[4:]) != 0 || binary.LittleEndian.Uint16(e[6:]) != 0 || binary.LittleEndian.Uint16(e[8:]) != binary.LittleEndian.Uint16(e[10:]) {
		return invalid("split-archive")
	}
	count := uint64(binary.LittleEndian.Uint16(e[10:]))
	length := uint64(binary.LittleEndian.Uint32(e[12:]))
	offset := uint64(binary.LittleEndian.Uint32(e[16:]))
	if count == 65535 || length == 0xffffffff || offset == 0xffffffff {
		if boundary < 20 {
			return invalid("zip64-locator")
		}
		locator := make([]byte, 20)
		if _, err := file.ReadAt(locator, boundary-20); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(locator) != 0x07064b50 || binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return invalid("zip64-locator")
		}
		position := binary.LittleEndian.Uint64(locator[8:])
		if position > uint64(boundary-20) || uint64(boundary-20)-position < 56 {
			return invalid("zip64-end-record")
		}
		record := make([]byte, 56)
		if _, err := file.ReadAt(record, int64(position)); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(record) != 0x06064b50 || binary.LittleEndian.Uint64(record[4:]) != 44 || binary.LittleEndian.Uint32(record[16:]) != 0 || binary.LittleEndian.Uint32(record[20:]) != 0 || binary.LittleEndian.Uint64(record[24:]) != binary.LittleEndian.Uint64(record[32:]) || position+56 != uint64(boundary-20) {
			return invalid("zip64-end-record")
		}
		count = binary.LittleEndian.Uint64(record[32:])
		length = binary.LittleEndian.Uint64(record[40:])
		offset = binary.LittleEndian.Uint64(record[48:])
		boundary = int64(position)
	}
	if count > 500000 || length > 128<<20 || offset > uint64(boundary) || length != uint64(boundary)-offset {
		return invalid("metadata-resource-limit")
	}
	position := offset
	actual := uint64(0)
	header := make([]byte, 46)
	for position < uint64(boundary) {
		if err := ctx.Err(); err != nil {
			return err
		}
		actual++
		if actual > 500000 || uint64(boundary)-position < 46 {
			return invalid("central-directory-count")
		}
		if _, err := file.ReadAt(header, int64(position)); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header) != 0x02014b50 {
			return invalid("central-directory-header")
		}
		entryLength := uint64(46) + uint64(binary.LittleEndian.Uint16(header[28:])) + uint64(binary.LittleEndian.Uint16(header[30:])) + uint64(binary.LittleEndian.Uint16(header[32:]))
		if entryLength > uint64(boundary)-position {
			return invalid("central-directory-length")
		}
		position += entryLength
	}
	if actual != count {
		return invalid("central-directory-count")
	}
	return ctx.Err()
}
