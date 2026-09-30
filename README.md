# Hub Clone Tool
![Build](https://github.com/paul-ridgway/hub-clone-tool/workflows/Build/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/paul-ridgway/hub-clone-tool.svg)](https://pkg.go.dev/github.com/paul-ridgway/hub-clone-tool)

![Demo](demo.gif "Demo")

Clones all repositories a user has access to from GitHub.

## Running

Install with [Go](https://go.dev/dl/):

```
go install github.com/paul-ridgway/hub-clone-tool@latest
```

And run with `hub-clone-tool`. For the shorter `hct` command, also install:

```
go install github.com/paul-ridgway/hub-clone-tool/cmd/hct@latest
```

Alternatively, download a prebuilt binary from the [releases page](https://github.com/paul-ridgway/hub-clone-tool/releases).

`git` must be available on your `PATH`.

> Versions up to 1.0.14 were published to npm as `hub-clone-tool`. The npm package is no longer updated; uninstall it with `npm un -g hub-clone-tool`.

## Development

```
./scripts/build.sh   # build into ./bin, then run ./bin/hct
./scripts/test-run.sh  # build and run against a temp dir, removed afterwards
go run .             # run from source
go test ./...        # run the tests
./scripts/local-install.sh
```

## Releasing

The tool is published as a Go module, so a release is just a `vX.Y.Z` git tag on `main`:

```
./scripts/release.sh v1.2.3
```

This runs the tests, then tags and pushes. The Release workflow then creates a GitHub release with prebuilt binaries and asks the Go module proxy to index the version, after which `go install ...@latest` and [pkg.go.dev](https://pkg.go.dev/github.com/paul-ridgway/hub-clone-tool) pick it up.

## Authentication
Authentication is (currently) by token, stored in the git settings.

To generate a token go to https://github.com/settings/tokens and create a personal access token with the following scopes:

- repo
- read:org

Store the generated token:

`git config --global --add github.apikey [token here]`

## Root Code Folder

By default the tool works out of the current working directory.

To ensure you always sync to the same folder it is advised to set a `code.home` global git config variable:

`git config --global --add code.home /home/paul/Documents/Code`

## Options

```
-d, --dir <path>     Directory to clone into (overrides code.home)
-c, --config <file>  Git config file to read github.apikey and code.home from
                     (default: your global git config)
-h, --help           Show help
```

## Cloning

Cloning is only supported via SSH (not HTTP/S) as there is no means to prompt for credentials.

## TODO
- Option to view lists of cloned/skipped repositories
