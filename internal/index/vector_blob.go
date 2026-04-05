package index

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func encodeVectorBlob(vector []float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, value := range vector {
		if err := binary.Write(buf, binary.LittleEndian, float32(value)); err != nil {
			return nil, fmt.Errorf("serialize vector: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// EncodeVectorBlob encodes float64 embeddings into the sqlite-vec blob format.
func EncodeVectorBlob(vector []float64) ([]byte, error) {
	return encodeVectorBlob(vector)
}

func decodeVectorBlob(blob []byte) ([]float64, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("invalid vector blob length %d", len(blob))
	}
	decoded := make([]float32, len(blob)/4)
	if err := binary.Read(bytes.NewReader(blob), binary.LittleEndian, decoded); err != nil {
		return nil, fmt.Errorf("decode vector blob: %w", err)
	}
	vector := make([]float64, len(decoded))
	for i, value := range decoded {
		vector[i] = float64(value)
	}
	return vector, nil
}

// DecodeVectorBlob decodes a sqlite-vec float32 blob into float64 values.
func DecodeVectorBlob(blob []byte) ([]float64, error) {
	return decodeVectorBlob(blob)
}

func cosineScoreFromDistance(distance float64) float64 {
	score := 1 - distance
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}
