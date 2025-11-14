- check whether each for statement runs at least 2 times.
- add the linter _gopls check_
- add the linter _modernize_. Call it with
  `go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -test ./...`.
  Run `go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -help` to see how to use it.
- execute fuzzing tests. We need to check if the architecture supports it (amd64 or arm64).
  Use the option --fuzztime=10s. For each Fuzz function create a gorrotine to execute it. Pass the option --run=^$
  to prevent other test from run.