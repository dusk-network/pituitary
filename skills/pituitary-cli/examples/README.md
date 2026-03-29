# Request Templates

These JSON files are starter templates for Pituitary commands that support `--request-file`.

Use them as copy-and-edit inputs:

1. Copy a template into the working repository.
2. Replace the example spec refs, doc refs, or paths with values from the target repo.
3. Run the matching command with `--request-file`.

Example:

```sh
cp skills/pituitary-cli/examples/compare-request.json .pituitary-compare-request.json
$EDITOR .pituitary-compare-request.json
pituitary compare-specs --request-file .pituitary-compare-request.json --format json
```

Do not assume the shipped example refs exist in every repository. They are placeholders drawn from Pituitary's own example corpus.
