# simpleenv

[![CI](https://github.com/edgarsilva/simpleenv/actions/workflows/ci.yml/badge.svg)](https://github.com/edgarsilva/simpleenv/actions/workflows/ci.yml)
[![Go Test](https://github.com/edgarsilva/simpleenv/actions/workflows/go-test.yml/badge.svg)](https://github.com/edgarsilva/simpleenv/actions/workflows/go-test.yml)

`simpleenv` maps environment variables into a Go struct and validates values using `env` struct tags.

## Install

```bash
go get github.com/edgarsilva/simpleenv
```

## Upgrade

```bash
go get github.com/edgarsilva/simpleenv@v1.1.1
go mod tidy
```

To verify the resolved version:

```bash
go list -m github.com/edgarsilva/simpleenv
```

### Migration Note (v1.1.1)

- Tagged env vars now fail when present with an empty value (`KEY=`) by default.
- If empty values are intentional, add `allowempty` to `string` or `encoding.TextUnmarshaler` fields.
- `allowempty` is invalid for numeric, boolean, and duration fields.

## Quick Start

```go
package main

import (
    "log"

    "github.com/edgarsilva/simpleenv"
)

type AppEnv struct {
    Environment string  `env:"ENVIRONMENT;oneof=development,test,staging,production"`
    Version     float64 `env:"VERSION;optional"`
    APIURL      string  `env:"API_URL;format=URL"`
    Concurrency int     `env:"CONCURRENCY;min=1;max=32"`

    // No env tag: ignored by simpleenv (useful for defaults/derived values).
    ServiceName string
}

func main() {
    cfg := AppEnv{ServiceName: "my-service"}

    if err := simpleenv.Load(&cfg); err != nil {
        log.Fatal(err)
    }
}
```

## Tag Format

Tag format is:

`env:"ENV_KEY;constraint1;constraint2"`

Examples:

- `env:"PORT;min=1;max=65535"`
- `env:"MODE;oneof=dev,test,prod"`
- `env:"PUBSUB_URL;regex='(http|https)://(localhost|127.0.0.1):[0-9]+'"`

## Supported Field Types

- `string`
- `bool`
- `int`
- `int64`
- `uint`
- `float64`
- `time.Duration`
- custom types implementing `encoding.TextUnmarshaler`

## Supported Constraints

- `optional`: allows env var to be missing.
- Missing optional env vars keep whatever value was already in the struct (or zero value if it started empty).
- `allowempty`: only for `string` or `encoding.TextUnmarshaler` fields; allows `MY_ENV_VAR=` when the key exists.
- `oneof=a,b,c`: value must match one option
- `min=n`: numeric value must be `>= n` (for `time.Duration`, use duration values like `500ms`, `2s`, `1m`)
- `max=n`: numeric value must be `<= n` (for `time.Duration`, use duration values like `500ms`, `2s`, `1m`)
- `regex=pattern`: value must match regex (single or double quoted patterns are supported)
- `format=...`: value must match one of the supported formats below

### Supported `format` Values

- `URL`: valid `http`/`https` URL
- `URI`: valid URI with a scheme
- `FILE`: existing file path
- `DIR`: existing directory path
- `HOSTPORT`: valid `host:port` value
- `UUID`: valid UUID (canonical hyphenated form)
- `IP`: valid IPv4 or IPv6 address
- `HEX`: hexadecimal string (`0-9`, `a-f`, `A-F`)
- `ALPHANUMERIC`: letters and numbers only
- `IDENTIFIER`: letters, numbers, `_`, and `-` only

Note: only one format value is allowed (`format=URL` is valid, `format=URL|FILE` is rejected).

## Behavior Notes

- `Load` requires a pointer to a struct: `simpleenv.Load(&cfg)`.
- Fields without an `env` tag are skipped.
- `optional` applies only when the env var is missing, not when it is empty (`MY_ENV_VAR=`).
- By default, if a tagged env var is present but empty (`MY_ENV_VAR=`), `Load` returns an error.
- Use `allowempty` only for `string` or `encoding.TextUnmarshaler` fields when empty values are intentional.
- `allowempty` is invalid for numeric, boolean, and duration fields; use `optional` when the env var may be missing.
- Unknown constraints return an error.
- Unknown `format=` values return an error.

## Error Shape

Validation/parse errors include field name, env key, invalid value, and expectation:

`invalid value for field "Concurrency" from ENV["CONCURRENCY"]: got "abc", expected a valid int`
