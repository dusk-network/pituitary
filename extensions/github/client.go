package github

import (
	"context"
	"fmt"
	"net/http"

	ghapi "github.com/google/go-github/v79/github"
)

type apiClient struct {
	client *ghapi.Client
}

func newIssueClient(options sourceOptions) (issueClient, error) {
	token, err := authToken(options)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{}
	if token != "" {
		httpClient.Transport = authTransport{base: http.DefaultTransport, token: token}
	}

	return &apiClient{client: ghapi.NewClient(httpClient)}, nil
}

func (c *apiClient) ListIssues(ctx context.Context, owner, repo string, options issueListOptions) ([]githubIssue, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("github client is not configured")
	}

	request := &ghapi.IssueListByRepoOptions{
		State:  options.State,
		Labels: options.Labels,
		ListOptions: ghapi.ListOptions{
			PerPage: options.PerPage,
		},
	}

	result := make([]githubIssue, 0)
	for {
		issues, response, err := c.client.Issues.ListByRepo(ctx, owner, repo, request)
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if issue == nil {
				continue
			}
			result = append(result, githubIssue{
				Number:        issue.GetNumber(),
				Title:         issue.GetTitle(),
				Body:          issue.GetBody(),
				HTMLURL:       issue.GetHTMLURL(),
				State:         issue.GetState(),
				Labels:        issueLabelNames(issue.Labels),
				Author:        issueAuthorLogin(issue.User),
				IsPullRequest: issue.PullRequestLinks != nil,
			})
		}
		if response == nil || response.NextPage == 0 {
			return result, nil
		}
		request.ListOptions.Page = response.NextPage
	}
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t authTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	clone := request.Clone(request.Context())
	clone.Header = clone.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return base.RoundTrip(clone)
}

func issueLabelNames(labels []*ghapi.Label) []string {
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		if label == nil {
			continue
		}
		result = append(result, label.GetName())
	}
	return result
}

func issueAuthorLogin(user *ghapi.User) string {
	if user == nil {
		return ""
	}
	return user.GetLogin()
}
