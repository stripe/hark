set quiet

export PATH := home_directory() + "/go/bin:" + env('PATH')

_default:
    just --list --unsorted

# run format, lint, and tests to prepare for CI
prepare: format lint test

# run all unit tests
test *args="./...":
    go test -race {{ args }}

# build the binary for local development
build:
    go build -o bin/hark .

# run the linter
lint:
    go fix -diff ./...
    go vet -diff -fix ./...
    golangci-lint run ./...

# try to fix any lint issues
lint-fix:
    go fix ./...
    go vet -fix ./...

# format all Go files
format:
    gofmt -s -w .

# verify formatting without modifying files
format-check:
    @test -z "$(gofmt -s -l .)" || (echo "Run 'just format' to fix:" && gofmt -s -l . && exit 1)

# install development tools
install:
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

dev *args:
    go run main.go {{ args }}

# trigger a release with the given version
[confirm("This will tag the latest commit and push that tag, kicking off the release workflow. Proceed (y/N)?")]
release version:
    {{ assert(version =~ "^\\d+\\.\\d+\\.\\d+$", "call this with a semver version, got \"" + version + "\"") }}

    {{ assert(`git status --porcelain` == "", "working tree is dirty; commit or stash first") }}

    git fetch --quiet origin # must fetch before checking freshness
    {{ assert(`git rev-parse HEAD` == `git rev-parse @{u}`, "HEAD does not match origin; pull or push before releasing") }}

    git tag -a "v{{ version }}" -m "v{{ version }}"
    git push origin "v{{ version }}"

    echo "Done! {{ version }} is on its way out."
