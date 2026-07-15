package codec_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"

	compression "github.com/skip-mev/slinky/abci/strategies/codec"
)

const benchmarkPayloadSize = 80 * 1024

var (
	benchmarkPayload    = makeBenchmarkPayload(benchmarkPayloadSize)
	benchmarkCompressed = mustCompress(benchmarkPayload)
	benchmarkResult     []byte
)

func makeBenchmarkPayload(size int) []byte {
	data := make([]byte, size)

	if _, err := rand.Read(data); err != nil {
		panic(err)
	}

	return data
}

func mustCompress(data []byte) []byte {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		panic(err)
	}
	defer encoder.Close()

	return encoder.EncodeAll(data, nil)
}

// Measures only the cost of constructing and closing a Zstd writer.
func BenchmarkZStdNewWriter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		writer, err := zstd.NewWriter(io.Discard)
		if err != nil {
			b.Fatal(err)
		}

		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// Measures writer construction plus streaming compression.
func BenchmarkZStdNewWriterAndCompress(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))

	for b.Loop() {
		var destination bytes.Buffer

		writer, err := zstd.NewWriter(&destination)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := writer.Write(benchmarkPayload); err != nil {
			writer.Close()
			b.Fatal(err)
		}

		if err := writer.Close(); err != nil {
			b.Fatal(err)
		}

		benchmarkResult = destination.Bytes()
	}
}

// Measures only the cost of constructing and closing a Zstd reader.
func BenchmarkZStdNewReader(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		reader, err := zstd.NewReader(bytes.NewReader(benchmarkCompressed))
		if err != nil {
			b.Fatal(err)
		}

		reader.Close()
	}
}

// Measures reader construction plus streaming decompression.
func BenchmarkZStdNewReaderAndDecompress(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))

	for b.Loop() {
		reader, err := zstd.NewReader(bytes.NewReader(benchmarkCompressed))
		if err != nil {
			b.Fatal(err)
		}

		output, err := io.ReadAll(reader)
		reader.Close()

		if err != nil {
			b.Fatal(err)
		}

		benchmarkResult = output
	}
}

// Measures the repository's public compressor implementation.
// On the unpatched branch, this uses the reusable encoder.
func BenchmarkZStdCompressorCompress(b *testing.B) {
	compressor := compression.NewZStdCompressor()

	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))
	b.ResetTimer()

	for b.Loop() {
		output, err := compressor.Compress(benchmarkPayload)
		if err != nil {
			b.Fatal(err)
		}

		benchmarkResult = output
	}
}

// Measures the repository's public decompressor implementation.
// On the unpatched branch, this uses DecodeAll with the reusable decoder.
func BenchmarkZStdCompressorDecompress(b *testing.B) {
	compressor := compression.NewZStdCompressor()

	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))
	b.ResetTimer()

	for b.Loop() {
		output, err := compressor.Decompress(benchmarkCompressed)
		if err != nil {
			b.Fatal(err)
		}

		benchmarkResult = output
	}
}
