package sentry

import (
	"context"
	"encoding/binary"

	sentrygo "github.com/getsentry/sentry-go"
)

var tracePayloadMagic = [5]byte{0xFF, 'S', 'E', 'N', '1'}

const tracePayloadHeaderSize = len(tracePayloadMagic) + 4

func WrapPayloadWithCurrentTrace(ctx context.Context, body []byte) []byte {
	if !Enabled() || ctx == nil {
		return body
	}
	span := sentrygo.SpanFromContext(ctx)
	if span == nil {
		return body
	}
	traceHeader := span.ToSentryTrace()
	baggageHeader := span.ToBaggage()
	if traceHeader == "" && baggageHeader == "" {
		return body
	}
	return encodeTracePayload(traceHeader, baggageHeader, body)
}

func encodeTracePayload(traceHeader, baggageHeader string, body []byte) []byte {
	encoded := make([]byte, 0, tracePayloadHeaderSize+len(traceHeader)+len(baggageHeader)+len(body))
	encoded = append(encoded, tracePayloadMagic[:]...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(traceHeader)))
	encoded = append(encoded, lengthBuf[:]...)
	encoded = append(encoded, traceHeader...)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(baggageHeader)))
	encoded = append(encoded, lengthBuf[:]...)
	encoded = append(encoded, baggageHeader...)
	encoded = append(encoded, body...)
	return encoded
}

func decodeTracePayload(payload []byte) (body []byte, traceHeader, baggageHeader string, hasTrace bool) {
	if len(payload) < tracePayloadHeaderSize {
		return payload, "", "", false
	}
	for i, b := range tracePayloadMagic {
		if payload[i] != b {
			return payload, "", "", false
		}
	}
	cursor := len(tracePayloadMagic)
	traceLen := int(binary.BigEndian.Uint16(payload[cursor : cursor+2]))
	cursor += 2
	if cursor+traceLen+2 > len(payload) {
		return payload, "", "", false
	}
	traceHeader = string(payload[cursor : cursor+traceLen])
	cursor += traceLen
	baggageLen := int(binary.BigEndian.Uint16(payload[cursor : cursor+2]))
	cursor += 2
	if cursor+baggageLen > len(payload) {
		return payload, "", "", false
	}
	baggageHeader = string(payload[cursor : cursor+baggageLen])
	cursor += baggageLen
	body = payload[cursor:]
	return body, traceHeader, baggageHeader, true
}
