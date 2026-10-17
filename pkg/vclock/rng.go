package vclock

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
)

// DeterministicRNG provides seed-reproducible pseudo-random generation and UUIDs.
// It uses an internal SplitMix64 / XorShift128+ engine to guarantee identical output
// across different CPU architectures and Go runtimes.
type DeterministicRNG struct {
	mu sync.Mutex
	s0 uint64
	s1 uint64
}

// NewDeterministicRNG initializes a reproducible RNG from an int64 seed.
func NewDeterministicRNG(seed int64) *DeterministicRNG {
	rng := &DeterministicRNG{}
	rng.Seed(seed)
	return rng
}

// Seed reinitializes the internal 128-bit state using SplitMix64.
func (r *DeterministicRNG) Seed(seed int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	smState := uint64(seed)
	if smState == 0 {
		smState = 0x853c49e6748fea9b
	}

	splitmix64 := func() uint64 {
		smState += 0x9e3779b97f4a7c15
		z := smState
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		return z ^ (z >> 31)
	}

	r.s0 = splitmix64()
	r.s1 = splitmix64()
}

// Uint64 generates a 64-bit pseudo-random unsigned integer via XorShift128+.
func (r *DeterministicRNG) Uint64() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.uint64Locked()
}

func (r *DeterministicRNG) uint64Locked() uint64 {
	s1 := r.s0
	s0 := r.s1
	r.s0 = s0
	s1 ^= s1 << 23
	r.s1 = s1 ^ s0 ^ (s1 >> 17) ^ (s0 >> 26)
	return r.s1 + s0
}

// Float64 returns a deterministic float in [0.0, 1.0).
func (r *DeterministicRNG) Float64() float64 {
	return float64(r.Uint64()>>11) / (1 << 53)
}

// Intn returns a deterministic integer in [0, n).
func (r *DeterministicRNG) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Uint64() % uint64(n))
}

// Read implements io.Reader to serve as an in-process drop-in for crypto/rand.
func (r *DeterministicRNG) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := len(p)
	offset := 0
	for offset < total {
		val := r.uint64Locked()
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], val)
		toCopy := 8
		if total-offset < 8 {
			toCopy = total - offset
		}
		copy(p[offset:], buf[:toCopy])
		offset += toCopy
	}
	return total, nil
}

// UUID generates an RFC 4122 variant 1, version 4 UUID string deterministically.
func (r *DeterministicRNG) UUID() string {
	var b [16]byte
	_, _ = r.Read(b[:])

	// Version 4: set bits 4..7 of byte 6 to 0100
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant 1: set bits 6..7 of byte 8 to 10
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Fork creates an independent child deterministic RNG keyed by a domain separator.
// This allows concurrent sub-agents to generate reproducible streams without mutex contention.
func (r *DeterministicRNG) Fork(domain string) *DeterministicRNG {
	r.mu.Lock()
	parentVal := r.uint64Locked()
	r.mu.Unlock()

	hasher := sha256.New()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], parentVal)
	hasher.Write(buf[:])
	hasher.Write([]byte(domain))
	digest := hasher.Sum(nil)

	childSeed := int64(binary.BigEndian.Uint64(digest[:8]))
	return NewDeterministicRNG(childSeed)
}

// ExportState returns the exact 128-bit internal state.
func (r *DeterministicRNG) ExportState() (uint64, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s0, r.s1
}

// RestoreState restores the exact 128-bit internal state.
func (r *DeterministicRNG) RestoreState(s0, s1 uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s0 = s0
	r.s1 = s1
}

// State returns a lightweight 64-bit hash of current RNG state.
func (r *DeterministicRNG) State() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s0 ^ r.s1
}
