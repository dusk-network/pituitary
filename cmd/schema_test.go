package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunSchemaCatalogJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runSchema([]string{"--format", "json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runSchema() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSchema() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Command string `json:"command"`
		} `json:"request"`
		Result struct {
			Commands []struct {
				Name             string   `json:"name"`
				Description      string   `json:"description"`
				SupportedFormats []string `json:"supported_formats"`
			} `json:"commands"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal schema catalog payload: %v", err)
	}
	if payload.Request.Command != "" {
		t.Fatalf("request.command = %q, want empty", payload.Request.Command)
	}
	if len(payload.Result.Commands) == 0 {
		t.Fatal("result.commands is empty, want command catalog")
	}
	var foundSchema bool
	for _, command := range payload.Result.Commands {
		if command.Name == "schema" {
			foundSchema = true
			if command.Description == "" || len(command.SupportedFormats) == 0 {
				t.Fatalf("schema catalog entry = %+v, want description and formats", command)
			}
		}
	}
	if !foundSchema {
		t.Fatalf("result.commands = %+v, want schema entry", payload.Result.Commands)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunSchemaCommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runSchema([]string{"compare-specs", "--format", "json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runSchema(compare-specs) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSchema(compare-specs) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Command string `json:"command"`
		} `json:"request"`
		Result struct {
			Name                string         `json:"name"`
			MutatesState        bool           `json:"mutates_state"`
			SupportsRequestFile bool           `json:"supports_request_file"`
			InputSchema         map[string]any `json:"input_schema"`
			OutputSchema        map[string]any `json:"output_schema"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal schema command payload: %v", err)
	}
	if payload.Request.Command != "compare-specs" || payload.Result.Name != "compare-specs" {
		t.Fatalf("request/result command mismatch: request=%+v result=%+v", payload.Request, payload.Result)
	}
	if payload.Result.MutatesState {
		t.Fatalf("mutates_state = true, want false")
	}
	if !payload.Result.SupportsRequestFile {
		t.Fatal("supports_request_file = false, want true")
	}
	inputProps, _ := payload.Result.InputSchema["properties"].(map[string]any)
	if _, ok := inputProps["spec_refs"]; !ok {
		t.Fatalf("input schema properties = %#v, want spec_refs", inputProps)
	}
	outputProps, _ := payload.Result.OutputSchema["properties"].(map[string]any)
	if _, ok := outputProps["comparison"]; !ok {
		t.Fatalf("output schema properties = %#v, want comparison", outputProps)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunSchemaCheckComplianceIncludesRelationOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runSchema([]string{"check-compliance", "--format", "json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runSchema(check-compliance) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSchema(check-compliance) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Name         string         `json:"name"`
			OutputSchema map[string]any `json:"output_schema"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal check-compliance schema payload: %v", err)
	}
	if payload.Result.Name != "check-compliance" {
		t.Fatalf("result.name = %q, want check-compliance", payload.Result.Name)
	}
	outputProps, _ := payload.Result.OutputSchema["properties"].(map[string]any)
	if _, ok := outputProps["relations"]; !ok {
		t.Fatalf("output schema properties = %#v, want relations", outputProps)
	}
	if _, ok := outputProps["relation_summary"]; !ok {
		t.Fatalf("output schema properties = %#v, want relation_summary", outputProps)
	}
	if _, ok := outputProps["discovery"]; !ok {
		t.Fatalf("output schema properties = %#v, want discovery", outputProps)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunVersionUsesPituitaryFormatEnv(t *testing.T) {
	t.Setenv(formatEnvVar, "json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runVersion([]string{}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runVersion() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runVersion() wrote unexpected stderr: %q", stderr.String())
	}

	var payload cliEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal version payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunVersionDefaultsToJSONWhenStdoutIsRedirected(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "version.out")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("os.Create() error = %v", err)
	}
	defer file.Close()

	var stderr bytes.Buffer
	exitCode := runVersion([]string{}, file, &stderr)
	if exitCode != 0 {
		t.Fatalf("runVersion() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runVersion() wrote unexpected stderr: %q", stderr.String())
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	var payload cliEnvelope
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("redirected version output is not JSON: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func mustWriteJSONFileCmd(t *testing.T, path string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T) error = %v", value, err)
	}
	mustWriteFileCmd(t, path, string(data))
}
