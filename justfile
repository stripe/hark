set quiet
set no-exit-message

export PATH := home_directory() + "/go/bin:" + env('PATH')

golangci_lint_version := "v2.11.4"

_default:
    just --list --unsorted

# run every check CI runs, so a green run here means a green run there
prepare: test format lint (zizmor "-qq")

# run all unit tests
test *args="./...":
    go test -race {{ args }}

# build the binary for local development
build:
    go build -o bin/hark .

# validate all 21 SDK channel branches against the local build, so a new rule cannot reject live data unnoticed
validate-sdks: build
    HARK_BIN="{{ justfile_directory() }}/bin/hark" potent run misc/validate-existing/validate-existing.plan.json

# run the linter
lint:
    go fix -diff ./...
    go vet -diff -fix ./...
    golangci-lint run ./...

# try to fix any lint issues
lint-fix:
    go fix ./...
    go vet -fix ./...

# audit the workflows and composite actions for security problems
zizmor *args:
    zizmor --min-severity high . {{ args }}

# format all Go files
format:
    gofmt -s -w .

# verify formatting without modifying files
format-check:
    @test -z "$(gofmt -s -l .)" || (echo "Run 'just format' to fix:" && gofmt -s -l . && exit 1)

# install development tools
install:
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ golangci_lint_version }}

dev *args:
    go run main.go {{ args }}

# trigger a release with the given version
[confirm("This will tag the latest commit and push that tag, kicking off the release workflow. Proceed (y/N)?")]
release version: validate-sdks
    {{ assert(version =~ "^\\d+\\.\\d+\\.\\d+$", "call this with a semver version, got \"" + version + "\"") }}

    {{ assert(`grep -c -i -E '^#+ *\[?unreleased' CHANGELOG.md || true` == "0", "CHANGELOG.md still has an Unreleased heading; retitle it to " + version + " first") }}

    {{ assert(`git status --porcelain` == "", "working tree is dirty; commit or stash first") }}

    git fetch --quiet origin # must fetch before checking freshness
    {{ assert(`git rev-parse HEAD` == `git rev-parse @{u}`, "HEAD does not match origin; pull or push before releasing") }}

    git tag -a "v{{ version }}" -m "v{{ version }}"
    git push origin "v{{ version }}"

    echo "Done! {{ version }} is on its way out."
