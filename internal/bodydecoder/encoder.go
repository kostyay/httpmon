package bodydecoder

import "errors"

// ErrNoEncoder is returned when no registered decoder also implements Encoder
// for the given content type.
var ErrNoEncoder = errors.New("no encoder matched content type")

// Encoder encodes a JSON body back into wire format.
// Decoders that support round-tripping implement both Decoder and Encoder.
type Encoder interface {
	CanEncode(contentType string) bool
	Encode(jsonBody []byte, contentType string, meta DecoderMetadata) ([]byte, error)
}
