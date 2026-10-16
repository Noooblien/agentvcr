package storage

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// CompressionAlgorithm defines the streaming compression type for trace logs.
type CompressionAlgorithm string

const (
	AlgorithmNone CompressionAlgorithm = "none"
	AlgorithmGzip CompressionAlgorithm = "gzip"
	AlgorithmZstd CompressionAlgorithm = "zstd" // Future native CGO/WASM driver
)

// CompressTracePayload compresses raw JSON bytes using standard streaming compression.
func CompressTracePayload(data []byte, alg CompressionAlgorithm) ([]byte, error) {
	switch alg {
	case AlgorithmNone:
		return data, nil
	case AlgorithmGzip:
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(data); err != nil {
			return nil, fmt.Errorf("gzip compression failed: %w", err)
		}
		if err := gw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", alg)
	}
}

// DecompressTracePayload transparently decompresses data based on magic byte signatures.
func DecompressTracePayload(data []byte) ([]byte, error) {
	// Gzip magic header: 0x1f, 0x8b
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize gzip reader: %w", err)
		}
		defer gr.Close()
		return io.ReadAll(gr)
	}
	// Plain JSON
	return data, nil
}
