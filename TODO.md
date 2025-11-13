- check whether each for statement runs at least 2 times.
- add the linter _gopls check_
- add the linter _modernize_. Call it with
  `go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -test ./...`.
  Run `go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -help` to see how to use it.
- execute fuzzing tests. We need to check if the architecture supports it (amd64 or arm64).
  Use the option --fuzztime=10s. For each Fuzz function create a gorrotine to execute it. Pass the option --run=^$
  to prevent other test from run.
- make possible to execute test for each specific operation using files in _testdata_ like the _result.txt_.
  For example, for executing the _tests_ operation with a timeout of 500ms we can add a file named _tests.txt_
  in the folder. In this file we specify the timeout. The tests verify if the folder contains a file named
  _tests.txt_ and use it to execute the _tests_ operation.
- use `go test -list='Test|Fuzz' -vet=off -json ./...` to test if there are tests.