package grok

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// grpcHeaderFields mirrors GrokWebBillingFetcher.grpcHeaderFields: HTTP
// response headers whose lowercased names start with "grpc-", with values
// trimmed and percent-decoded.
func grpcHeaderFields(h http.Header) map[string]string {
	fields := map[string]string{}
	for key, values := range h {
		lower := strings.ToLower(key)
		if !strings.HasPrefix(lower, "grpc-") {
			continue
		}
		fields[lower] = percentDecode(strings.TrimSpace(strings.Join(values, ", ")))
	}
	return fields
}

// grpcWebTrailerFields mirrors GrokWebBillingFetcher.grpcWebTrailerFields:
// "key: value" lines inside grpc-web trailer frames (flag bit 0x80). Keys are
// trimmed/lowercased, values trimmed and percent-decoded.
func grpcWebTrailerFields(data []byte) map[string]string {
	fields := map[string]string{}
	i := 0
	for i+5 <= len(data) {
		flags := data[i]
		length := int(data[i+1])<<24 | int(data[i+2])<<16 | int(data[i+3])<<8 | int(data[i+4])
		start := i + 5
		end := start + length
		if length < 0 || end > len(data) {
			break // malformed framing: keep the fields parsed so far
		}
		if flags&0x80 != 0 {
			for _, line := range strings.Split(string(data[start:end]), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				sep := strings.IndexByte(line, ':')
				if sep < 0 {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(line[:sep]))
				fields[key] = percentDecode(strings.TrimSpace(line[sep+1:]))
			}
		}
		i = end
	}
	return fields
}

// validateGRPCStatusFields mirrors GrokWebBillingFetcher.validateGRPCStatusFields:
// a non-zero grpc-status becomes an RPC error carrying grpc-message.
func validateGRPCStatusFields(fields map[string]string) error {
	raw, ok := fields["grpc-status"]
	if !ok {
		return nil
	}
	status, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || status == 0 {
		return nil
	}
	return errRPCFailed(status, fields["grpc-message"])
}

// validateGRPCWebTrailers mirrors GrokWebBillingFetcher.validateGRPCWebTrailers.
func validateGRPCWebTrailers(data []byte) error {
	return validateGRPCStatusFields(grpcWebTrailerFields(data))
}

// grpcWebDataFrames mirrors GrokWebBillingFetcher.grpcWebDataFrames: the
// payloads of frames whose flag bit 0x80 is clear (data frames). Malformed
// framing yields no frames, matching the Swift behavior.
func grpcWebDataFrames(data []byte) [][]byte {
	var frames [][]byte
	i := 0
	for i < len(data) {
		if i+5 > len(data) {
			return nil
		}
		flags := data[i]
		length := int(data[i+1])<<24 | int(data[i+2])<<16 | int(data[i+3])<<8 | int(data[i+4])
		start := i + 5
		end := start + length
		if length < 0 || end > len(data) {
			return nil
		}
		if flags&0x80 == 0 {
			frames = append(frames, data[start:end])
		}
		i = end
	}
	return frames
}

// looksLikeProtobufPayload mirrors GrokWebBillingFetcher.looksLikeProtobufPayload.
func looksLikeProtobufPayload(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	fieldNumber := data[0] >> 3
	wireType := data[0] & 0x07
	return fieldNumber > 0 && (wireType == 0 || wireType == 1 || wireType == 2 || wireType == 5)
}

// percentDecode mirrors Swift's removingPercentEncoding (which does not treat
// "+" as a space), falling back to "" on malformed input.
func percentDecode(s string) string {
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return ""
}
