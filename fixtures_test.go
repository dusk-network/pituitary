package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type bootstrapExpectations struct {
	Specs        []specExpectation    `json:"specs"`
	OverlapPairs []overlapExpectation `json:"overlap_pairs"`
	Docs         []docExpectation     `json:"docs"`
}

type specExpectation struct {
	Ref        string   `json:"ref"`
	Path       string   `json:"path"`
	Status     string   `json:"status"`
	Domain     string   `json:"domain"`
	DependsOn  []string `json:"depends_on"`
	Supersedes []string `json:"supersedes"`
	AppliesTo  []string `json:"applies_to"`
}

type overlapExpectation struct {
	Left            string   `json:"left"`
	Right           string   `json:"right"`
	SharedAppliesTo []string `json:"shared_applies_to"`
}

type docExpectation struct {
	Path           string   `json:"path"`
	Classification string   `json:"classification"`
	MustContain    []string `json:"must_contain"`
	MustNotContain []string `json:"must_not_contain"`
}

type specFixture struct {
	ID         string
	Status     string
	Domain     string
	Body       string
	DependsOn  []string
	Supersedes []string
	AppliesTo  []string
}

var quotedValuePattern = regexp.MustCompile(`"([^"]+)"`)

func TestBootstrapFixtureWorkspace(t *testing.T) {
	expectations := loadBootstrapExpectations(t)

	specPaths := mustCollectFiles(t, "specs", ".toml")
	if got, want := len(specPaths), len(expectations.Specs); got != want {
		t.Fatalf("spec fixture count = %d, want %d", got, want)
	}

	docPaths := append(
		mustCollectFiles(t, filepath.Join("docs", "guides"), ".md"),
		mustCollectFiles(t, filepath.Join("docs", "runbooks"), ".md")...,
	)
	if got, want := len(docPaths), len(expectations.Docs); got != want {
		t.Fatalf("doc fixture count = %d, want %d", got, want)
	}

	for _, expected := range expectations.Specs {
		spec := mustLoadSpecFixture(t, expected.Path)
		if spec.ID != expected.Ref {
			t.Fatalf("%s id = %q, want %q", expected.Path, spec.ID, expected.Ref)
		}
		if spec.Status != expected.Status {
			t.Fatalf("%s status = %q, want %q", expected.Path, spec.Status, expected.Status)
		}
		if spec.Domain != expected.Domain {
			t.Fatalf("%s domain = %q, want %q", expected.Path, spec.Domain, expected.Domain)
		}
		if !equalStringSlices(spec.DependsOn, expected.DependsOn) {
			t.Fatalf("%s depends_on = %v, want %v", expected.Path, spec.DependsOn, expected.DependsOn)
		}
		if !equalStringSlices(spec.Supersedes, expected.Supersedes) {
			t.Fatalf("%s supersedes = %v, want %v", expected.Path, spec.Supersedes, expected.Supersedes)
		}
		if !equalStringSlices(spec.AppliesTo, expected.AppliesTo) {
			t.Fatalf("%s applies_to = %v, want %v", expected.Path, spec.AppliesTo, expected.AppliesTo)
		}

		bodyPath := filepath.Join(filepath.Dir(expected.Path), spec.Body)
		if _, err := os.Stat(bodyPath); err != nil {
			t.Fatalf("%s body %q is missing: %v", expected.Path, bodyPath, err)
		}
	}
}

func TestBootstrapExpectedRelationshipsAndDocs(t *testing.T) {
	expectations := loadBootstrapExpectations(t)

	specsByRef := make(map[string]specFixture, len(expectations.Specs))
	for _, expected := range expectations.Specs {
		specsByRef[expected.Ref] = mustLoadSpecFixture(t, expected.Path)
	}

	for _, expected := range expectations.OverlapPairs {
		left, ok := specsByRef[expected.Left]
		if !ok {
			t.Fatalf("missing left overlap spec %q", expected.Left)
		}
		right, ok := specsByRef[expected.Right]
		if !ok {
			t.Fatalf("missing right overlap spec %q", expected.Right)
		}
		if left.Domain != right.Domain {
			t.Fatalf("overlap pair %s/%s domain mismatch: %q vs %q", expected.Left, expected.Right, left.Domain, right.Domain)
		}
		if !equalStringSlices(left.AppliesTo, expected.SharedAppliesTo) {
			t.Fatalf("left overlap applies_to = %v, want %v", left.AppliesTo, expected.SharedAppliesTo)
		}
		if !equalStringSlices(right.AppliesTo, expected.SharedAppliesTo) {
			t.Fatalf("right overlap applies_to = %v, want %v", right.AppliesTo, expected.SharedAppliesTo)
		}
	}

	for _, expected := range expectations.Docs {
		content := mustReadFile(t, expected.Path)
		for _, needle := range expected.MustContain {
			if !strings.Contains(content, needle) {
				t.Fatalf("%s (%s) does not contain expected text %q", expected.Path, expected.Classification, needle)
			}
		}
		for _, needle := range expected.MustNotContain {
			if strings.Contains(content, needle) {
				t.Fatalf("%s (%s) unexpectedly contains %q", expected.Path, expected.Classification, needle)
			}
		}
	}
}

func TestMalformedFixtureHome(t *testing.T) {
	readme := mustReadFile(t, "testdata/README.md")
	if !strings.Contains(readme, "invalid-spec-bundle") {
		t.Fatalf("testdata/README.md does not document malformed fixture home")
	}

	specPath := filepath.Join("testdata", "invalid-spec-bundle", "missing-body", "spec.toml")
	spec := mustLoadSpecFixture(t, specPath)
	bodyPath := filepath.Join(filepath.Dir(specPath), spec.Body)
	_, err := os.Stat(bodyPath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed fixture body %q stat error = %v, want %v", bodyPath, err, os.ErrNotExist)
	}
}

func loadBootstrapExpectations(t *testing.T) bootstrapExpectations {
	t.Helper()

	var expectations bootstrapExpectations
	data := mustReadFile(t, filepath.Join("testdata", "bootstrap_expectations.json"))
	if err := json.Unmarshal([]byte(data), &expectations); err != nil {
		t.Fatalf("unmarshal bootstrap expectations: %v", err)
	}
	return expectations
}

func mustCollectFiles(t *testing.T, root, suffix string) []string {
	t.Helper()

	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			paths = append(paths, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return paths
}

func mustLoadSpecFixture(t *testing.T, path string) specFixture {
	t.Helper()

	var spec specFixture
	var activeArrayKey string

	for _, rawLine := range strings.Split(mustReadFile(t, path), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if activeArrayKey != "" {
			if line == "]" {
				activeArrayKey = ""
				continue
			}
			assignSpecField(&spec, activeArrayKey, parseQuotedValues(t, line))
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("%s contains malformed line %q", path, line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if value == "[" {
			activeArrayKey = key
			assignSpecField(&spec, key, nil)
			continue
		}

		if strings.HasPrefix(value, "[") {
			assignSpecField(&spec, key, parseQuotedValues(t, value))
			continue
		}

		assignSpecScalar(&spec, key, parseQuotedScalar(t, value))
	}

	return spec
}

func assignSpecScalar(spec *specFixture, key, value string) {
	switch key {
	case "id":
		spec.ID = value
	case "status":
		spec.Status = value
	case "domain":
		spec.Domain = value
	case "body":
		spec.Body = value
	}
}

func assignSpecField(spec *specFixture, key string, values []string) {
	switch key {
	case "depends_on":
		spec.DependsOn = append(spec.DependsOn, values...)
	case "supersedes":
		spec.Supersedes = append(spec.Supersedes, values...)
	case "applies_to":
		spec.AppliesTo = append(spec.AppliesTo, values...)
	}
}

func parseQuotedScalar(t *testing.T, value string) string {
	t.Helper()

	matches := quotedValuePattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		t.Fatalf("scalar value %q does not contain exactly one quoted string", value)
	}
	return matches[1]
}

func parseQuotedValues(t *testing.T, value string) []string {
	t.Helper()

	matches := quotedValuePattern.FindAllStringSubmatch(value, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
