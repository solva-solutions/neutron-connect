package codec

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"

	cometabci "github.com/cometbft/cometbft/abci/types"
	"github.com/klauspost/compress/zstd"

	vetypes "github.com/skip-mev/slinky/abci/ve/types"
)

var (
	ErrDecompressionLimit = fmt.Errorf("decompression limit reached")
)

// VoteExtensionCodec is the interface for encoding / decoding vote extensions.
//
//go:generate mockery --name VoteExtensionCodec --filename vote_extension_codec.go
type VoteExtensionCodec interface {
	// Encode encodes the vote extension into a byte array.
	Encode(ve vetypes.OracleVoteExtension) ([]byte, error)

	// Decode decodes the vote extension from a byte array.
	Decode([]byte) (vetypes.OracleVoteExtension, error)
}

// ExtendedCommitCodec is the interface for encoding / decoding extended commit info.
//
//go:generate mockery --name ExtendedCommitCodec --filename extended_commit_codec.go
type ExtendedCommitCodec interface {
	// Encode encodes the extended commit info into a byte array.
	Encode(cometabci.ExtendedCommitInfo) ([]byte, error)

	// Decode decodes the extended commit info from a byte array.
	Decode([]byte) (cometabci.ExtendedCommitInfo, error)
}

// NewDefaultVoteExtensionCodec returns a new DefaultVoteExtensionCodec.
func NewDefaultVoteExtensionCodec() *DefaultVoteExtensionCodec {
	return &DefaultVoteExtensionCodec{}
}

// DefaultVoteExtensionCodec is the default implementation of VoteExtensionCodec. It uses the
// vanilla implementations of Unmarshal / Marshal under the hood.
type DefaultVoteExtensionCodec struct{}

func (codec *DefaultVoteExtensionCodec) Encode(ve vetypes.OracleVoteExtension) ([]byte, error) {
	return ve.Marshal()
}

func (codec *DefaultVoteExtensionCodec) Decode(bz []byte) (vetypes.OracleVoteExtension, error) {
	var ve vetypes.OracleVoteExtension
	return ve, ve.Unmarshal(bz)
}

// NewVoteExtensionCodecWithSizeCheck returns a new VoteExtensionCodecWithSizeCheck.
func NewVoteExtensionCodecWithSizeCheck() *VoteExtensionCodecWithSizeCheck {
	return &VoteExtensionCodecWithSizeCheck{}
}

// VoteExtensionCodecWithSizeCheck is an implementation of VoteExtensionCodec with a size check
// on Decoding level. For Encode it uses the vanilla Marshal implementation. For Decode it has
// an additional Encode and Compare step for checking whether the incoming bytes are the same as
// the decoded and encoded bytes. This makes sure that the vote extension doesn't have any
// extraneous fields.
type VoteExtensionCodecWithSizeCheck struct{}

func (codec *VoteExtensionCodecWithSizeCheck) Encode(ve vetypes.OracleVoteExtension) ([]byte, error) {
	return ve.Marshal()
}

func (codec *VoteExtensionCodecWithSizeCheck) Decode(bz []byte) (vetypes.OracleVoteExtension, error) {
	var ve vetypes.OracleVoteExtension
	if err := ve.Unmarshal(bz); err != nil {
		return vetypes.OracleVoteExtension{}, fmt.Errorf("failed to unmarshal vote extension: %w", err)
	}

	remarshaled, err := ve.Marshal()
	if err != nil {
		return vetypes.OracleVoteExtension{}, fmt.Errorf("failed to remarshal vote extension for size check: %w", err)
	}
	if len(bz) != len(remarshaled) {
		return vetypes.OracleVoteExtension{}, fmt.Errorf("incoming bytes size doesn't match the remarshaled bytes size: %d != %d", len(bz), len(remarshaled))
	}

	return ve, nil
}

type Compressor interface {
	Compress([]byte) ([]byte, error)
	Decompress([]byte) ([]byte, error)
}

// ZLibCompressor is a Compressor that uses zlib to compress / decompress byte arrays, this object is not thread-safe.
type ZLibCompressor struct {
	// decompressLimit is the maximum number of bytes that can be decompressed.
	// if <=0, no limit is applied.
	decompressLimit int
}

// NewZLibCompressorWithLimit returns a new zlibDecompressor with the given decompression limit.
func NewZLibCompressorWithLimit(limit int) *ZLibCompressor {
	return &ZLibCompressor{decompressLimit: limit}
}

// NewZLibCompressor returns a new zlibDecompressor with no limit.
func NewZLibCompressor() *ZLibCompressor {
	return NewZLibCompressorWithLimit(0)
}

// Compress compresses the given byte array using zlib. It returns an error if the compression fails.
// This function is not thread-safe, and uses zlib.BestCompression as the compression level.
func (c *ZLibCompressor) Compress(bz []byte) ([]byte, error) {
	var b bytes.Buffer

	if len(bz) > c.decompressLimit && c.decompressLimit > 0 {
		return nil, fmt.Errorf("zlib compression limit reached")
	}

	// we use the best compression level as size reduction is prioritized
	w := zlib.NewWriter(&b)
	defer w.Close()

	// write and flush the buffer
	if _, err := w.Write(bz); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// Decompress decompresses the given byte array using zlib. It returns an error if the decompression fails.
func (c *ZLibCompressor) Decompress(bz []byte) ([]byte, error) {
	if len(bz) == 0 {
		return nil, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(bz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var r io.Reader = zr
	if c.decompressLimit > 0 {
		r = newLimitReaderWithError(r, c.decompressLimit)
	}
	return io.ReadAll(r)
}

// ZStdCompressor is a Compressor that uses zstd to compress / decompress byte arrays, this object is thread-safe.
type ZStdCompressor struct {
	// decompressLimit is the maximum number of bytes that can be decompressed.
	// if <=0, no limit is applied.
	decompressLimit int
}

func NewZStdCompressor() *ZStdCompressor {
	return &ZStdCompressor{}
}

// NewZStdCompressorWithLimit returns a new ZStdDecompressor with the given decompression limit.
func NewZStdCompressorWithLimit(limit int) *ZStdCompressor {
	return &ZStdCompressor{decompressLimit: limit}
}

func (c *ZStdCompressor) Compress(bz []byte) ([]byte, error) {
	var b bytes.Buffer

	if len(bz) > c.decompressLimit && c.decompressLimit > 0 {
		return nil, fmt.Errorf("zstd compression limit reached")
	}

	// we use the best compression level as size reduction is prioritized
	w, err := zstd.NewWriter(&b)
	if err != nil {
		return nil, err
	}
	defer w.Close()

	// write and flush the buffer
	if _, err := w.Write(bz); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (c *ZStdCompressor) Decompress(bz []byte) ([]byte, error) {
	if len(bz) == 0 {
		return nil, nil
	}
	zr, err := zstd.NewReader(bytes.NewReader(bz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var r io.Reader = zr
	if c.decompressLimit > 0 {
		r = newLimitReaderWithError(r, c.decompressLimit)
	}
	return io.ReadAll(r)
}

// CompressionVoteExtensionCodec is a VoteExtensionCodec that uses compression to encode / decode, given a
// VoteExtensionCodec that can encode / decode uncompressed vote extensions.
type CompressionVoteExtensionCodec struct {
	codec      VoteExtensionCodec
	compressor Compressor
}

// NewCompressionVoteExtensionCodec returns a new CompressionVoteExtensionCodec given an underlying codec.
func NewCompressionVoteExtensionCodec(codec VoteExtensionCodec, compressor Compressor) *CompressionVoteExtensionCodec {
	return &CompressionVoteExtensionCodec{
		codec:      codec,
		compressor: compressor,
	}
}

// Encode returns the encoded vote extension using the codec's Encode method and then compresses the result.
// This implementation uses zstd compression.
func (codec *CompressionVoteExtensionCodec) Encode(ve vetypes.OracleVoteExtension) ([]byte, error) {
	bz, err := codec.codec.Encode(ve)
	if err != nil {
		return nil, err
	}

	return codec.compressor.Compress(bz)
}

// Decode decompresses the vote extension using zstd and then decodes the result using the codec's Decode method.
func (codec *CompressionVoteExtensionCodec) Decode(bz []byte) (vetypes.OracleVoteExtension, error) {
	// decompress first
	bz, err := codec.compressor.Decompress(bz)
	if err != nil {
		return vetypes.OracleVoteExtension{}, err
	}

	return codec.codec.Decode(bz)
}

// DefaultExtendedCommitCodec is the default implementation of ExtendedCommitCodec. It uses the
// vanilla implementations of Unmarshal / Marshal under the hood.
type DefaultExtendedCommitCodec struct{}

// NewDefaultExtendedCommitCodec returns a new DefaultExtendedCommitCodec.
func NewDefaultExtendedCommitCodec() *DefaultExtendedCommitCodec {
	return &DefaultExtendedCommitCodec{}
}

func (codec *DefaultExtendedCommitCodec) Encode(extendedCommitInfo cometabci.ExtendedCommitInfo) ([]byte, error) {
	return extendedCommitInfo.Marshal()
}

func (codec *DefaultExtendedCommitCodec) Decode(bz []byte) (cometabci.ExtendedCommitInfo, error) {
	if len(bz) == 0 {
		return cometabci.ExtendedCommitInfo{}, nil
	}

	var extendedCommitInfo cometabci.ExtendedCommitInfo
	return extendedCommitInfo, extendedCommitInfo.Unmarshal(bz)
}

// CompressionExtendedCommitCodec is a ExtendedCommitCodec that uses compression to encode / decode, given a
// ExtendedCommitCodec that can encode / decode uncompressed extended commit info.
type CompressionExtendedCommitCodec struct {
	codec      ExtendedCommitCodec
	compressor Compressor
}

// NewCompressionExtendedCommitCodec returns a new CompressionExtendedCommitCodec given an underlying codec.
func NewCompressionExtendedCommitCodec(codec ExtendedCommitCodec, compressor Compressor) *CompressionExtendedCommitCodec {
	return &CompressionExtendedCommitCodec{
		codec:      codec,
		compressor: compressor,
	}
}

// Encode returns the encoded extended commit info using the codec's Encode method and then compresses the result.
// This implementation uses zstd compression.
func (codec *CompressionExtendedCommitCodec) Encode(extendedCommitInfo cometabci.ExtendedCommitInfo) ([]byte, error) {
	bz, err := codec.codec.Encode(extendedCommitInfo)
	if err != nil {
		return nil, err
	}

	return codec.compressor.Compress(bz)
}

// Decode decompresses the extended commit info using zstd and then decodes the result using the codec's Decode method.
func (codec *CompressionExtendedCommitCodec) Decode(bz []byte) (cometabci.ExtendedCommitInfo, error) {
	// decompress first
	bz, err := codec.compressor.Decompress(bz)
	if err != nil {
		return cometabci.ExtendedCommitInfo{}, err
	}

	return codec.codec.Decode(bz)
}

// limitReaderWithError is a io.Reader that reads up to n bytes from the underlying reader.
// Unlike io.LimitReader, this reader returns ErrZLibDecompressionLimit if >n bytes were read.
type limitReaderWithError struct {
	r io.Reader
	n int
}

func newLimitReaderWithError(r io.Reader, n int) *limitReaderWithError {
	return &limitReaderWithError{r: r, n: n}
}

func (lr *limitReaderWithError) Read(p []byte) (int, error) {
	if lr.n <= 0 {
		var probe [1]byte
		// read one extra byte to detect(trigger) EOF error
		_, err := lr.r.Read(probe[:])
		if err != nil {
			return 0, err
		}
		return 0, ErrDecompressionLimit
	}
	if len(p) > lr.n {
		p = p[:lr.n]
	}
	n, err := lr.r.Read(p)
	lr.n -= n
	return n, err
}
