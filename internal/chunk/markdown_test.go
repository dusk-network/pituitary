package chunk

import (
	"reflect"
	"testing"
)

func TestMarkdownSplitsHeadingAwareSections(t *testing.T) {
	t.Parallel()

	sections := Markdown("Spec Title", `
Preamble paragraph.

## Requirements

- First requirement.

### Burst Handling

More detail.

## Rollout

Ship it.
`)

	want := []Section{
		{
			Heading: "Spec Title",
			Body:    "Preamble paragraph.",
		},
		{
			Heading: "Requirements",
			Body:    "- First requirement.",
		},
		{
			Heading: "Requirements / Burst Handling",
			Body:    "More detail.",
		},
		{
			Heading: "Rollout",
			Body:    "Ship it.",
		},
	}

	if !reflect.DeepEqual(sections, want) {
		t.Fatalf("Markdown() = %#v, want %#v", sections, want)
	}
}

func TestMarkdownUsesTitleWhenBodyHasNoHeadings(t *testing.T) {
	t.Parallel()

	sections := Markdown("Spec Title", "Body text without headings.\n")
	want := []Section{{
		Heading: "Spec Title",
		Body:    "Body text without headings.",
	}}
	if !reflect.DeepEqual(sections, want) {
		t.Fatalf("Markdown() = %#v, want %#v", sections, want)
	}
}

func TestMarkdownSkipsEmptyHeadingBlocks(t *testing.T) {
	t.Parallel()

	sections := Markdown("Doc Title", `
# Doc Title

## Empty

## Filled

Actual content.
`)

	want := []Section{{
		Heading: "Doc Title / Filled",
		Body:    "Actual content.",
	}}
	if !reflect.DeepEqual(sections, want) {
		t.Fatalf("Markdown() = %#v, want %#v", sections, want)
	}
}
