package bodydecoder

import (
	"encoding/binary"
	"fmt"
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
	ct := stripParams(contentType)
	for _, t := range grpcWebContentTypes {
		if ct == t {
			return true
		}
	}
	return false
}

func (d *GRPCWebDecoder) Decode(body []byte, meta DecoderMetadata) (string, string, error) {
	payload, notes, err := extractDataFrames(body)
	if err != nil {
		return "", "", fmt.Errorf("grpc-web frame: %w", err)
	}

	var result string
	var resultCT string

	if len(payload) > 0 {
		decoded, ct, decErr := d.Proto.Decode(payload, meta)
		if decErr != nil {
			return "", "", fmt.Errorf("grpc-web payload decode: %w", decErr)
		}
		result = decoded
		resultCT = ct
	} else {
		result = "{}"
		resultCT = "application/json"
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
