package bodydecoder

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// grpcWebContentTypes are the MIME types handled by the gRPC-Web decoder.
var grpcWebContentTypes = []string{
	"application/grpc-web",
	"application/grpc-web+proto",
}

// GRPCWebDecoder strips gRPC-Web frame headers and delegates the
// extracted payload to a protobuf decoder.
type GRPCWebDecoder struct {
	Proto *RawProtobufDecoder
}

func (d *GRPCWebDecoder) CanDecode(contentType string) bool {
	return matchesContentType(contentType, grpcWebContentTypes)
}

func (d *GRPCWebDecoder) CanEncode(contentType string) bool {
	return d.CanDecode(contentType)
}

func (d *GRPCWebDecoder) Encode(jsonBody []byte, contentType string, meta DecoderMetadata) ([]byte, error) {
	payload, err := d.Proto.Encode(jsonBody, contentType, meta)
	if err != nil {
		return nil, err
	}
	if len(payload) > math.MaxUint32 {
		return nil, fmt.Errorf("payload too large for gRPC-Web frame: %d bytes", len(payload))
	}
	// Wrap in a single gRPC-Web data frame: flag(1) + length(4) + payload.
	frame := make([]byte, frameHeaderLen+len(payload))
	frame[0] = flagData
	binary.BigEndian.PutUint32(frame[1:frameHeaderLen], uint32(len(payload))) // #nosec G115 -- guarded above
	copy(frame[frameHeaderLen:], payload)

	// Preserve non-data frames (trailers) from the original body.
	frame = append(frame, extractNonDataFrames(meta.OriginalBody)...)
	return frame, nil
}

func (d *GRPCWebDecoder) Decode(body []byte, meta DecoderMetadata) (string, string, error) {
	payload, notes, err := extractDataFrames(body)
	if err != nil {
		return "", "", fmt.Errorf("grpc-web frame: %w", err)
	}

	result, resultCT := "{}", "application/json"
	if len(payload) > 0 {
		decoded, ct, decErr := d.Proto.Decode(payload, meta)
		if decErr != nil {
			return "", "", fmt.Errorf("grpc-web payload decode: %w", decErr)
		}
		result, resultCT = decoded, ct
	}

	if len(notes) > 0 {
		result += "\n\n// " + strings.Join(notes, "\n// ")
	}
	return result, resultCT, nil
}

const (
	frameHeaderLen = 5
	flagData       = 0x00
	flagCompressed = 0x01
	flagTrailers   = 0x80
)

// extractNonDataFrames returns the raw bytes of all non-data frames (trailers,
// compressed, etc.) from a gRPC-Web body. Used to preserve trailers during re-encode.
func extractNonDataFrames(body []byte) []byte {
	var out []byte
	remaining := body
	for len(remaining) >= frameHeaderLen {
		flag := remaining[0]
		length := binary.BigEndian.Uint32(remaining[1:frameHeaderLen])
		end := frameHeaderLen + int(length)
		if end > len(remaining) {
			end = len(remaining) // truncated frame — preserve whatever bytes exist
		}
		if flag != flagData {
			out = append(out, remaining[:end]...)
		}
		remaining = remaining[end:]
	}
	return out
}

// extractDataFrames parses gRPC-Web framing and returns the concatenated
// data frame payloads. Compressed and trailer frames are skipped with notes.
func extractDataFrames(body []byte) (payload []byte, notes []string, err error) {
	remaining := body
	for len(remaining) > 0 {
		if len(remaining) < frameHeaderLen {
			return nil, nil, fmt.Errorf("truncated frame header: %d bytes remaining", len(remaining))
		}

		flag := remaining[0]
		length := binary.BigEndian.Uint32(remaining[1:frameHeaderLen])
		remaining = remaining[frameHeaderLen:]

		truncated := false
		frameData := remaining
		if uint64(len(remaining)) < uint64(length) {
			// Truncated frame — decode what we have.
			truncated = true
			notes = append(notes, fmt.Sprintf("truncated frame: expected %d bytes, got %d", length, len(remaining)))
		} else {
			frameData = remaining[:length]
			remaining = remaining[length:]
		}

		switch {
		case flag == flagData:
			payload = append(payload, frameData...)
		case flag == flagCompressed:
			notes = append(notes, fmt.Sprintf("[compressed gRPC payload, %d bytes]", length))
		case flag&flagTrailers != 0:
			// Trailer frame — skip.
		default:
			notes = append(notes, fmt.Sprintf("[unknown frame flag 0x%02x, %d bytes]", flag, length))
		}

		if truncated {
			break
		}
	}
	return payload, notes, nil
}
