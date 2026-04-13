package index

import ststore "github.com/dusk-network/stroma/store"

func encodeVectorBlob(vector []float64) ([]byte, error) {
	return ststore.EncodeVectorBlob(vector)
}

// EncodeVectorBlob encodes float64 embeddings into the sqlite-vec blob format.
func EncodeVectorBlob(vector []float64) ([]byte, error) {
	return ststore.EncodeVectorBlob(vector)
}

func decodeVectorBlob(blob []byte) ([]float64, error) {
	return ststore.DecodeVectorBlob(blob)
}

// DecodeVectorBlob decodes a sqlite-vec float32 blob into float64 values.
func DecodeVectorBlob(blob []byte) ([]float64, error) {
	return ststore.DecodeVectorBlob(blob)
}

func cosineScoreFromDistance(distance float64) float64 {
	return ststore.CosineScoreFromDistance(distance)
}
