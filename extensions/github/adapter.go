package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/sdk"
)

const (
	adapterName = "github"
	kindIssue   = "issue"
)

var defaultSpecLabels = map[string]struct{}{
	"adr":      {},
	"proposal": {},
	"rfc":      {},
	"spec":     {},
	"specs":    {},
}

func init() {
	sdk.Register(adapterName, func() sdk.Adapter {
		return &adapter{clientFactory: newIssueClient}
	})
}

type adapter struct {
	clientFactory issueClientFactory
}

func (a *adapter) Load(ctx context.Context, cfg sdk.SourceConfig) (*sdk.AdapterResult, error) {
	if strings.TrimSpace(cfg.Kind) != kindIssue {
		return nil, fmt.Errorf("unsupported kind %q", cfg.Kind)
	}

	options, err := parseSourceOptions(cfg.Options)
	if err != nil {
		return nil, err
	}

	clientFactory := a.clientFactory
	if clientFactory == nil {
		clientFactory = newIssueClient
	}
	client, err := clientFactory(options)
	if err != nil {
		return nil, err
	}

	issues, err := client.ListIssues(ctx, options.owner, options.repo, issueListOptions{
		Labels:  options.labels,
		State:   options.state,
		PerPage: options.perPage,
	})
	if err != nil {
		return nil, err
	}

	result := &sdk.AdapterResult{
		Specs: make([]sdk.SpecRecord, 0, len(issues)),
		Docs:  make([]sdk.DocRecord, 0, len(issues)),
	}
	for _, issue := range issues {
		if issue.IsPullRequest {
			continue
		}
		if issueIsSpec(issue) {
			result.Specs = append(result.Specs, specRecordFromIssue(options.repoFullName, issue))
			continue
		}
		result.Docs = append(result.Docs, docRecordFromIssue(options.repoFullName, issue))
	}

	return result, nil
}

type sourceOptions struct {
	repoFullName string
	owner        string
	repo         string
	labels       []string
	state        string
	apiKeyEnv    string
	perPage      int
}

func parseSourceOptions(options map[string]any) (sourceOptions, error) {
	if len(options) == 0 {
		return sourceOptions{}, fmt.Errorf("options.repo is required")
	}

	const (
		defaultState   = "open"
		defaultPerPage = 100
	)

	parsed := sourceOptions{
		state:   defaultState,
		perPage: defaultPerPage,
	}

	allowed := map[string]struct{}{
		"api_key_env": {},
		"labels":      {},
		"per_page":    {},
		"repo":        {},
		"state":       {},
	}
	for key := range options {
		if _, ok := allowed[key]; !ok {
			return sourceOptions{}, fmt.Errorf("unsupported option %q", key)
		}
	}

	repoValue, ok := options["repo"]
	if !ok {
		return sourceOptions{}, fmt.Errorf("options.repo is required")
	}
	repo, err := optionString(repoValue)
	if err != nil {
		return sourceOptions{}, fmt.Errorf("options.repo: %w", err)
	}
	owner, name, err := splitRepo(repo)
	if err != nil {
		return sourceOptions{}, fmt.Errorf("options.repo: %w", err)
	}
	parsed.repoFullName = repo
	parsed.owner = owner
	parsed.repo = name

	if value, ok := options["labels"]; ok {
		labels, err := optionStringSlice(value)
		if err != nil {
			return sourceOptions{}, fmt.Errorf("options.labels: %w", err)
		}
		sort.Strings(labels)
		parsed.labels = labels
	}
	if value, ok := options["state"]; ok {
		state, err := optionString(value)
		if err != nil {
			return sourceOptions{}, fmt.Errorf("options.state: %w", err)
		}
		state = strings.ToLower(strings.TrimSpace(state))
		switch state {
		case "open", "closed", "all":
			parsed.state = state
		default:
			return sourceOptions{}, fmt.Errorf("options.state: unsupported value %q", state)
		}
	}
	if value, ok := options["api_key_env"]; ok {
		apiKeyEnv, err := optionString(value)
		if err != nil {
			return sourceOptions{}, fmt.Errorf("options.api_key_env: %w", err)
		}
		parsed.apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	}
	if value, ok := options["per_page"]; ok {
		perPage, err := optionInt(value)
		if err != nil {
			return sourceOptions{}, fmt.Errorf("options.per_page: %w", err)
		}
		if perPage <= 0 || perPage > 100 {
			return sourceOptions{}, fmt.Errorf("options.per_page: must be between 1 and 100")
		}
		parsed.perPage = perPage
	}

	return parsed, nil
}

func splitRepo(repo string) (string, string, error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("expected owner/repo")
	}
	return parts[0], parts[1], nil
}

func optionString(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", value)
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("value must not be empty")
	}
	return text, nil
}

func optionStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		result := make([]string, 0, len(typed))
		for i, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item %d: expected string, got %T", i, item)
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string array, got %T", value)
	}
}

func optionInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("expected integer, got %v", typed)
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, fmt.Errorf("parse integer: %w", err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}

func issueIsSpec(issue githubIssue) bool {
	title := strings.TrimSpace(issue.Title)
	if strings.HasPrefix(strings.ToUpper(title), "RFC ") || strings.HasPrefix(strings.ToUpper(title), "RFC:") {
		return true
	}
	for _, label := range issue.Labels {
		normalized := normalizeLabel(label)
		if _, ok := defaultSpecLabels[normalized]; ok {
			return true
		}
		if strings.HasPrefix(normalized, "spec:") || strings.HasPrefix(normalized, "rfc:") || strings.HasPrefix(normalized, "adr:") {
			return true
		}
	}
	return false
}

func specRecordFromIssue(repo string, issue githubIssue) sdk.SpecRecord {
	labels := normalizedIssueLabels(issue.Labels)
	metadata := issueMetadata(repo, issue, labels)
	body := issueBodyText(issue)

	return sdk.SpecRecord{
		Ref:         issueRef(repo, issue.Number),
		Kind:        sdk.ArtifactKindSpec,
		Title:       strings.TrimSpace(issue.Title),
		Status:      specStatus(issue, labels),
		Domain:      issueDomain(repo, labels),
		Authors:     issueAuthors(issue),
		Tags:        labels,
		SourceRef:   issueSourceRef(repo, issue),
		BodyFormat:  sdk.BodyFormatMarkdown,
		BodyText:    body,
		ContentHash: issueContentHash(issue, labels),
		Metadata:    metadata,
	}
}

func docRecordFromIssue(repo string, issue githubIssue) sdk.DocRecord {
	labels := normalizedIssueLabels(issue.Labels)
	return sdk.DocRecord{
		Ref:         issueRef(repo, issue.Number),
		Kind:        sdk.ArtifactKindDoc,
		Title:       strings.TrimSpace(issue.Title),
		SourceRef:   issueSourceRef(repo, issue),
		BodyFormat:  sdk.BodyFormatMarkdown,
		BodyText:    issueBodyText(issue),
		ContentHash: issueContentHash(issue, labels),
		Metadata:    issueMetadata(repo, issue, labels),
	}
}

func issueRef(repo string, number int) string {
	return fmt.Sprintf("github-issue://%s/%d", repo, number)
}

func issueSourceRef(repo string, issue githubIssue) string {
	if strings.TrimSpace(issue.HTMLURL) != "" {
		return issue.HTMLURL
	}
	return (&url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   fmt.Sprintf("/%s/issues/%d", repo, issue.Number),
	}).String()
}

func issueBodyText(issue githubIssue) string {
	title := strings.TrimSpace(issue.Title)
	body := strings.TrimSpace(issue.Body)
	if body == "" {
		return "# " + title
	}
	return "# " + title + "\n\n" + body
}

func issueAuthors(issue githubIssue) []string {
	if strings.TrimSpace(issue.Author) == "" {
		return nil
	}
	return []string{issue.Author}
}

func normalizedIssueLabels(labels []string) []string {
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		normalized := normalizeLabel(label)
		if normalized == "" {
			continue
		}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func normalizeLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func specStatus(issue githubIssue, labels []string) string {
	hasLabel := func(target string) bool {
		for _, label := range labels {
			if label == target {
				return true
			}
		}
		return false
	}

	switch {
	case hasLabel("deprecated"):
		return sdk.StatusDeprecated
	case hasLabel("superseded"):
		return sdk.StatusSuperseded
	case hasLabel("accepted"), hasLabel("implemented"):
		return sdk.StatusAccepted
	case hasLabel("draft"):
		return sdk.StatusDraft
	case strings.EqualFold(issue.State, "closed"):
		return sdk.StatusAccepted
	case hasLabel("review"), strings.HasPrefix(strings.ToUpper(strings.TrimSpace(issue.Title)), "RFC"):
		return sdk.StatusReview
	default:
		return sdk.StatusDraft
	}
}

func issueDomain(repo string, labels []string) string {
	for _, label := range labels {
		for _, prefix := range []string{"domain:", "area:", "component:"} {
			if strings.HasPrefix(label, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(label, prefix))
			}
		}
	}
	if _, name, err := splitRepo(repo); err == nil {
		return name
	}
	return repo
}

func issueMetadata(repo string, issue githubIssue, labels []string) map[string]string {
	return map[string]string{
		"issue_author":   issue.Author,
		"issue_number":   strconv.Itoa(issue.Number),
		"issue_state":    strings.TrimSpace(issue.State),
		"issue_url":      issueSourceRef(repo, issue),
		"labels":         strings.Join(labels, ","),
		"repo":           repo,
		"source_adapter": adapterName,
		"source_kind":    kindIssue,
	}
}

func issueContentHash(issue githubIssue, labels []string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		strconv.Itoa(issue.Number),
		strings.TrimSpace(issue.Title),
		strings.TrimSpace(issue.Body),
		strings.TrimSpace(issue.State),
		strings.Join(labels, ","),
	}, "\n")))
	return hex.EncodeToString(hash[:])
}

type issueClientFactory func(sourceOptions) (issueClient, error)

type issueClient interface {
	ListIssues(ctx context.Context, owner, repo string, options issueListOptions) ([]githubIssue, error)
}

type issueListOptions struct {
	Labels  []string
	State   string
	PerPage int
}

type githubIssue struct {
	Number        int
	Title         string
	Body          string
	HTMLURL       string
	State         string
	Labels        []string
	Author        string
	IsPullRequest bool
}

func authToken(options sourceOptions) (string, error) {
	if strings.TrimSpace(options.apiKeyEnv) == "" {
		return "", nil
	}
	token := strings.TrimSpace(os.Getenv(options.apiKeyEnv))
	if token == "" {
		return "", fmt.Errorf("environment variable %q is not set", options.apiKeyEnv)
	}
	return token, nil
}
