package fileid

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Reader implements low level binary deserialization for TL.
type Reader struct {
	Buf []byte
	Off int
}

func (r *Reader) ReadUint32() (uint32, error) {
	if r.Off+4 > len(r.Buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(r.Buf[r.Off : r.Off+4])
	r.Off += 4
	return v, nil
}

func (r *Reader) ReadUint64() (uint64, error) {
	if r.Off+8 > len(r.Buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint64(r.Buf[r.Off : r.Off+8])
	r.Off += 8
	return v, nil
}

func (r *Reader) ReadLong() (int64, error) {
	v, err := r.ReadUint64()
	return int64(v), err
}

func (r *Reader) ReadBytes() ([]byte, error) {
	if r.Off >= len(r.Buf) {
		return nil, io.ErrUnexpectedEOF
	}
	first := r.Buf[r.Off]
	r.Off++
	var l int
	if first <= maxSmallStringLength {
		l = int(first)
	} else if first == firstLongStringByte {
		if r.Off+3 > len(r.Buf) {
			return nil, io.ErrUnexpectedEOF
		}
		l = int(r.Buf[r.Off]) | int(r.Buf[r.Off+1])<<8 | int(r.Buf[r.Off+2])<<16
		r.Off += 3
	} else {
		return nil, fmt.Errorf("invalid string header: %d", first)
	}
	if r.Off+l > len(r.Buf) {
		return nil, io.ErrUnexpectedEOF
	}
	res := make([]byte, l)
	copy(res, r.Buf[r.Off:r.Off+l])
	r.Off += l
	// Skip padding
	padding := nearestPaddedValueLength(l + 1)
	if first == firstLongStringByte {
		padding = nearestPaddedValueLength(l + 4)
	}
	// The difference between padding and overall length of serialized string is the padding bytes count
	padLen := padding - (l + 1)
	if first == firstLongStringByte {
		padLen = padding - (l + 4)
	}
	r.Off += padLen
	return res, nil
}

func (r *Reader) ReadString() (string, error) {
	b, err := r.ReadBytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func base64Decode(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.StdEncoding.DecodeString(s)
}

func rleDecode(s []byte) ([]byte, error) {
	var r []byte
	for i := 0; i < len(s); i++ {
		cur := s[i]
		if cur == 0 {
			if i+1 >= len(s) {
				return nil, fmt.Errorf("invalid RLE data: trailing zero")
			}
			count := s[i+1]
			i++
			r = append(r, make([]byte, count)...)
		} else {
			r = append(r, cur)
		}
	}
	return r, nil
}

// DecodeFileID parses a file_id string into a FileID struct.
func DecodeFileID(s string) (FileID, error) {
	var f FileID
	decoded, err := base64Decode(s)
	if err != nil {
		return f, fmt.Errorf("base64 decode: %w", err)
	}
	decompressed, err := rleDecode(decoded)
	if err != nil {
		return f, fmt.Errorf("rle decode: %w", err)
	}

	if len(decompressed) < 9 { // min length: typeID(4) + DC(4) + version(1)
		return f, fmt.Errorf("buffer too short: %d", len(decompressed))
	}

	// The last byte of the decompressed buffer is the persistentIDVersion
	version := decompressed[len(decompressed)-1]
	if version != persistentIDVersion {
		// Try to proceed but keep in mind version mismatch
	}

	r := &Reader{Buf: decompressed[:len(decompressed)-1]}

	typeVal, err := r.ReadUint32()
	if err != nil {
		return f, fmt.Errorf("read typeID: %w", err)
	}

	hasWebLocation := (typeVal & webLocationFlag) != 0
	hasReference := (typeVal & fileReferenceFlag) != 0
	f.Type = Type(typeVal &^ (webLocationFlag | fileReferenceFlag))

	dcVal, err := r.ReadUint32()
	if err != nil {
		return f, fmt.Errorf("read DC: %w", err)
	}
	f.DC = int(dcVal)

	if hasReference {
		ref, err := r.ReadBytes()
		if err != nil {
			return f, fmt.Errorf("read file reference: %w", err)
		}
		f.FileReference = ref
	}

	if hasWebLocation {
		urlVal, err := r.ReadString()
		if err != nil {
			return f, fmt.Errorf("read URL: %w", err)
		}
		f.URL = urlVal
		return f, nil
	}

	idVal, err := r.ReadLong()
	if err != nil {
		return f, fmt.Errorf("read ID: %w", err)
	}
	f.ID = idVal

	accessHashVal, err := r.ReadLong()
	if err != nil {
		return f, fmt.Errorf("read AccessHash: %w", err)
	}
	f.AccessHash = accessHashVal

	return f, nil
}
