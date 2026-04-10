# Minimal Benchmark Cases

These cases are the intentionally small golden dataset for issue `#273`.

- They run against the shipped fixture workspace in this repo.
- They are meant to catch obvious regressions and compare prompt/runtime changes quickly.
- They are not the broader model matrix or parameter sweep tracked separately in `#285`.

Run the harness with:

```sh
go run ./cmd/bench --format text
go run ./cmd/bench --format json
```

Notes:

- Prompt and response byte counts are reported when `runtime.analysis` is configured and the harness can observe `/chat/completions` traffic.
- The impact case uses the shipped `analyze-impact` baseline even though the case name stays aligned with the planned `analyze-impact-severity` slice.
