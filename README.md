# simpleenv

`simpleenv` maps environment variables into a Go struct and validates values using `env` struct tags.

## Install

```bash
go get github.com/edgarsilva/simpleenv
```

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
- `int`
- `float64`

## Supported Constraints

- `optional`: env var may be unset
- `oneof=a,b,c`: value must match one option
- `min=n`: numeric value must be `>= n`
- `max=n`: numeric value must be `<= n`
- `regex=pattern`: value must match regex (single or double quoted patterns are supported)
- `format=URL`: value must be a valid `http`/`https` URL

## Behavior Notes

- `Load` requires a pointer to a struct: `simpleenv.Load(&cfg)`.
- Fields without an `env` tag are skipped.
- `optional` only applies when the env var is truly unset.
  - If env var is set to an empty string, it is treated as a provided value.
- Unknown constraints return an error.
- Unknown `format=` values return an error.

## Error Shape

Validation/parse errors include field name, env key, invalid value, and expectation:

`invalid value for field "Concurrency" from ENV["CONCURRENCY"]: got "abc", expected a valid int`
