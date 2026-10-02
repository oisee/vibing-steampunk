// A module of its own, like .github/ci/metrics, so the leak scanner never
// reaches vsp's go.mod or its release build. Go skips directories that start
// with a dot, so `go build ./...` at the repository root does not see this.
module github.com/oisee/vibing-steampunk/.github/ci/leakscan

go 1.24.0
