package simpleenv

import (
	"os"
	"testing"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	prev, hadPrev := os.LookupEnv(key)
	_ = os.Unsetenv(key)

	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv(key, prev)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestLoadOptionalIntMissingSkipsParsing(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_OPTIONAL_INT;optional"`
	}

	unsetEnv(t, "SIMPLEENV_TEST_OPTIONAL_INT")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if c.Concurrency != 0 {
		t.Fatalf("expected zero value for optional missing int, got %d", c.Concurrency)
	}
}

func TestLoadOptionalFloatMissingSkipsParsing(t *testing.T) {
	type cfg struct {
		Version float64 `env:"SIMPLEENV_TEST_OPTIONAL_FLOAT;optional"`
	}

	unsetEnv(t, "SIMPLEENV_TEST_OPTIONAL_FLOAT")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if c.Version != 0 {
		t.Fatalf("expected zero value for optional missing float, got %f", c.Version)
	}
}

func TestLoadOptionalWithMinMissingSkipsValidation(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_OPTIONAL_MIN;optional;min=1"`
	}

	unsetEnv(t, "SIMPLEENV_TEST_OPTIONAL_MIN")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLoadRequiredMissingReturnsError(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_REQUIRED_MISSING;min=1"`
	}

	unsetEnv(t, "SIMPLEENV_TEST_REQUIRED_MISSING")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected error for missing required env var, got nil")
	}
}

func TestLoadOptionalPresentInvalidValueReturnsError(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_OPTIONAL_INVALID;optional"`
	}

	t.Setenv("SIMPLEENV_TEST_OPTIONAL_INVALID", "abc")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected parsing error for present invalid optional int value, got nil")
	}
}

func TestLoadStructValueReturnsError(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_STRUCT_VALUE;optional"`
	}

	var c cfg
	err := Load(c)
	if err == nil {
		t.Fatal("expected error when passing struct by value, got nil")
	}
}

func TestLoadNilInputReturnsError(t *testing.T) {
	err := Load(nil)
	if err == nil {
		t.Fatal("expected error when passing nil, got nil")
	}
}

func TestLoadUnknownFormatConstraintReturnsError(t *testing.T) {
	type cfg struct {
		Token string `env:"SIMPLEENV_TEST_UNKNOWN_FORMAT;format=TOKEN;oneof=abc,def"`
	}

	t.Setenv("SIMPLEENV_TEST_UNKNOWN_FORMAT", "abc")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected error for unknown format constraint, got nil")
	}
}

func TestLoadWithWhitespaceInTagOptions(t *testing.T) {
	type cfg struct {
		Name string `env:" SIMPLEENV_TEST_TAG_SPACES ; optional ; oneof=foo,bar "`
	}

	t.Setenv("SIMPLEENV_TEST_TAG_SPACES", "foo")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error with whitespace-trimmed tag options, got %v", err)
	}

	if c.Name != "foo" {
		t.Fatalf("expected parsed value 'foo', got %q", c.Name)
	}
}

func TestLoadSkipsFieldsWithoutEnvTag(t *testing.T) {
	type cfg struct {
		Name         string `env:"SIMPLEENV_TEST_WITH_TAG"`
		DefaultValue string
	}

	t.Setenv("SIMPLEENV_TEST_WITH_TAG", "from-env")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error when untagged field exists, got %v", err)
	}

	if c.Name != "from-env" {
		t.Fatalf("expected Name from env, got %q", c.Name)
	}

	if c.DefaultValue != "" {
		t.Fatalf("expected untagged field to keep zero value, got %q", c.DefaultValue)
	}
}
