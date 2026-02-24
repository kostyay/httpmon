package bodydecoder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

// protobufContentTypes are the MIME types handled by the raw wire decoder.
var protobufContentTypes = []string{
	"application/protobuf",
	"application/x-protobuf",
	"application/x-google-protobuf",
	"application/grpc",
	"application/grpc+proto",
}

// RawProtobufDecoder decodes protobuf wire format into a JSON-like
// representation using field numbers as keys. When ProtoReg is set and
// the request has a gRPC path, it attempts named message decode first.
type RawProtobufDecoder struct {
	ProtoReg *ProtoRegistry
}

func (d *RawProtobufDecoder) CanDecode(contentType string) bool {
	ct := stripParams(contentType)
	for _, t := range protobufContentTypes {
		if ct == t {
			return true
		}
	}
	return false
}

func (d *RawProtobufDecoder) Decode(body []byte, meta DecoderMetadata) (string, string, error) {
	// Try named decode if proto registry is available and we have a gRPC path.
	if d.ProtoReg != nil && d.ProtoReg.HasMethods() && meta.RequestPath != "" {
		if decoded, err := d.ProtoReg.DecodeNamed(body, meta.RequestPath, meta.IsRequest); err == nil {
			return decoded, "application/json", nil
		}
		// Fall through to raw wire decode on failure.
	}

	return d.decodeRaw(body)
}

// decodeRaw performs raw wire-format decoding (field numbers as keys).
func (d *RawProtobufDecoder) decodeRaw(body []byte) (string, string, error) {
	fields, err := decodeWireFields(body)
	if err != nil {
		return "", "", fmt.Errorf("protobuf wire decode: %w", err)
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("protobuf json marshal: %w", err)
	}
	return string(out), "application/json", nil
}

// decodeWireFields parses protobuf wire format and returns an ordered map
// of field_number → value(s). Repeated fields produce arrays.
func decodeWireFields(b []byte) (orderedFields, error) {
	fields := orderedFields{}
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, fmt.Errorf("invalid tag at offset %d", len(b))
		}
		b = b[n:]

		var val any
		var consumed int

		switch wtype {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid varint for field %d", num)
			}
			consumed = n
			val = v

		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid fixed32 for field %d", num)
			}
			consumed = n
			// Show both uint and float interpretations if float looks meaningful.
			if f := math.Float32frombits(v); isReasonableFloat(float64(f)) {
				val = f
			} else {
				val = v
			}

		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid fixed64 for field %d", num)
			}
			consumed = n
			if f := math.Float64frombits(v); isReasonableFloat(f) {
				val = f
			} else {
				val = v
			}

		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return nil, fmt.Errorf("invalid bytes for field %d", num)
			}
			consumed = n
			val = interpretBytes(v)

		case protowire.StartGroupType:
			v, n := protowire.ConsumeGroup(num, b)
			if n < 0 {
				return nil, fmt.Errorf("invalid group for field %d", num)
			}
			consumed = n
			nested, err := decodeWireFields(v)
			if err != nil {
				return nil, fmt.Errorf("group field %d: %w", num, err)
			}
			val = nested

		default:
			return nil, fmt.Errorf("unknown wire type %d for field %d", wtype, num)
		}

		b = b[consumed:]
		fields = fields.append(num, val)
	}
	return fields, nil
}

// interpretBytes tries to decode a bytes field: first as a nested message,
// then as UTF-8 string, finally as base64.
func interpretBytes(b []byte) any {
	// Try nested message.
	if nested, err := decodeWireFields(b); err == nil && len(nested) > 0 {
		return nested
	}
	// Try UTF-8 string (reject if contains control chars other than \t \n \r).
	if isReadableString(b) {
		return string(b)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// isReadableString checks if bytes form valid, human-readable UTF-8.
func isReadableString(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	// Must be valid UTF-8.
	return len(b) == len([]rune(string(b)))
}

// isReasonableFloat returns true if the float value looks like a real number
// (not NaN, not Inf, and has a magnitude that suggests intentional use).
func isReasonableFloat(f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	abs := math.Abs(f)
	return abs > 1e-10 && abs < 1e15
}

// stripParams removes MIME type parameters (e.g. "; charset=utf-8").
func stripParams(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(strings.ToLower(ct))
}

// orderedFields preserves field insertion order for JSON output.
// Repeated field numbers produce JSON arrays.
type orderedFields []fieldEntry

type fieldEntry struct {
	Key string
	Val any
}

func (o orderedFields) append(num protowire.Number, val any) orderedFields {
	key := fmt.Sprintf("%d", num)
	for i, e := range o {
		if e.Key == key {
			// Convert to array if not already.
			switch existing := e.Val.(type) {
			case []any:
				o[i].Val = append(existing, val)
			default:
				o[i].Val = []any{existing, val}
			}
			return o
		}
	}
	return append(o, fieldEntry{Key: key, Val: val})
}

func (o orderedFields) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, _ := json.Marshal(e.Key)
		buf.Write(keyJSON)
		buf.WriteByte(':')
		valJSON, err := json.Marshal(e.Val)
		if err != nil {
			return nil, err
		}
		buf.Write(valJSON)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}
