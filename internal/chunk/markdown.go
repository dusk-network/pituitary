package chunk

import stchunk "github.com/dusk-network/stroma/chunk"

// Section is one heading-aware Markdown chunk.
type Section = stchunk.Section

// Markdown splits Markdown into heading-aware sections.
func Markdown(title, body string) []Section {
	return stchunk.Markdown(title, body)
}
