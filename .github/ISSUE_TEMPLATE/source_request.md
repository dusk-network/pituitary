---
name: Source request
about: Request support for a new source of specs, docs, or decisions
title: "[source] "
labels: ["enhancement", "extension"]
---

## What system holds your specs, docs, or decisions?

For example: GitHub issues, GitLab work items, Jira tickets, Notion pages, Confluence, JSON config files, PDF documents.

## How do you access it today?

API, CLI export, file download, browser only? Include links to docs if available.

## What kind of records does it contain?

Are these structured specs (with status, dependencies, ownership)? Decision records? Free-form documentation? A mix?

## What would you expect the config to look like?

Sketch what you imagine in `pituitary.toml`. For example:

```toml
[[sources]]
name = "my-source"
adapter = "???"
kind = "???"

[sources.options]
# what options would make sense here?
```

## Additional context

Anything else: how many records, how often they change, whether you need offline/cached access.
