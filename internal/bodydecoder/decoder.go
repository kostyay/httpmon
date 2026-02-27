// Package bodydecoder provides a pluggable body decoding pipeline.
// Decoders are tried in registration order; first CanDecode match wins.
package bodydecoder

import "errors"

// ErrNoDecoder is returned when no registered decoder matches the content type.
var ErrNoDecoder = errors.New("no decoder matched content type")

// DecoderMetadata carries per-request context that decoders may use
// for message type resolution (e.g. gRPC service/method from path).
type DecoderMetadata struct {
	RequestPath  string // e.g. /api/v1/package.Service/Method
	IsRequest    bool   // true = request body, false = response body
	OriginalBody []byte // set during re-encode so encoders can preserve non-data frames
}

// Decoder decodes a body from wire format into a human-readable string.
type Decoder interface {
	// CanDecode reports whether this decoder handles the given content type.
	CanDecode(contentType string) bool

	// Decode decodes body bytes and returns the decoded text plus a
	// resultContentType hint for downstream syntax highlighting
	// (e.g. "application/json").
	Decode(body []byte, metadata DecoderMetadata) (decoded string, resultContentType string, err error)
}

// Registry holds an ordered list of decoders and routes decode requests
// to the first matching decoder.
type Registry struct {
	decoders []Decoder
}

// NewRegistry creates a Registry with the given decoders tried in order.
func NewRegistry(decoders ...Decoder) *Registry {
	return &Registry{decoders: decoders}
}

// Decode finds the first decoder that can handle contentType and delegates to it.
// Returns ErrNoDecoder if no decoder matches.
func (r *Registry) Decode(body []byte, contentType string, meta DecoderMetadata) (string, string, error) {
	for _, d := range r.decoders {
		if d.CanDecode(contentType) {
			return d.Decode(body, meta)
		}
	}
	return "", "", ErrNoDecoder
}

// Encode finds the first decoder that also implements Encoder for the given
// content type and delegates to it. Returns ErrNoEncoder if none match.
func (r *Registry) Encode(jsonBody []byte, contentType string, meta DecoderMetadata) ([]byte, error) {
	for _, d := range r.decoders {
		if enc, ok := d.(Encoder); ok && enc.CanEncode(contentType) {
			return enc.Encode(jsonBody, contentType, meta)
		}
	}
	return nil, ErrNoEncoder
}
