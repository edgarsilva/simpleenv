package simpleenv

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type customToken string

func (c *customToken) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}

	*c = customToken("token:" + string(text))
	return nil
}

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

func TestLoadEmptyEnvTagValueReturnsError(t *testing.T) {
	type cfg struct {
		APITestNoEnvNameInTag string `env:""`
	}

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected error for empty env tag value, got nil")
	}
}

func TestLoadMalformedEnvTagReturnsError(t *testing.T) {
	field := reflect.StructField{
		Name: "APITestNoEnvNameInTag",
		Tag:  reflect.StructTag(`env:`),
	}

	_, err := parseEnvTag(field)
	if err == nil {
		t.Fatal("expected error for malformed env tag, got nil")
	}
}

func TestLoadUnknownConstraintReturnsError(t *testing.T) {
	type cfg struct {
		Name string `env:"SIMPLEENV_TEST_UNKNOWN_CONSTRAINT;nope=value"`
	}

	t.Setenv("SIMPLEENV_TEST_UNKNOWN_CONSTRAINT", "john")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected error for unknown constraint, got nil")
	}
}

func TestLoadRegexConstraintSupportsQuotedPattern(t *testing.T) {
	type cfg struct {
		PubsubHostURL string `env:"SIMPLEENV_TEST_REGEX_QUOTED;regex='(http|https)://(localhost|127.0.0.1):[0-9]+'"`
	}

	t.Setenv("SIMPLEENV_TEST_REGEX_QUOTED", "http://localhost:8085")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected no error for quoted regex pattern, got %v", err)
	}
}

func TestLoadErrorMessageIncludesFieldEnvAndExpected(t *testing.T) {
	type cfg struct {
		Concurrency int `env:"SIMPLEENV_TEST_ERROR_SHAPE;min=1"`
	}

	t.Setenv("SIMPLEENV_TEST_ERROR_SHAPE", "abc")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected validation/parsing error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "field \"Concurrency\"") {
		t.Fatalf("expected error to include field name, got %q", errStr)
	}
	if !strings.Contains(errStr, "ENV[\"SIMPLEENV_TEST_ERROR_SHAPE\"]") {
		t.Fatalf("expected error to include env key, got %q", errStr)
	}
	if !strings.Contains(errStr, "expected") {
		t.Fatalf("expected error to include expected constraint, got %q", errStr)
	}
}

func TestLoadFormatURIValid(t *testing.T) {
	type cfg struct {
		DatabaseURI string `env:"SIMPLEENV_TEST_FORMAT_URI;format=URI"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_URI", "postgres://localhost:5432/mydb")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid URI format to pass, got %v", err)
	}
}

func TestLoadFormatFILEValid(t *testing.T) {
	type cfg struct {
		ConfigPath string `env:"SIMPLEENV_TEST_FORMAT_FILE;format=FILE"`
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "simpleenv-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = tmpFile.Close()

	t.Setenv("SIMPLEENV_TEST_FORMAT_FILE", tmpFile.Name())

	var c cfg
	err = Load(&c)
	if err != nil {
		t.Fatalf("expected valid FILE format to pass, got %v", err)
	}
}

func TestLoadFormatDIRValid(t *testing.T) {
	type cfg struct {
		WorkDir string `env:"SIMPLEENV_TEST_FORMAT_DIR;format=DIR"`
	}

	path := t.TempDir()
	t.Setenv("SIMPLEENV_TEST_FORMAT_DIR", path)

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid DIR format to pass, got %v", err)
	}
}

func TestLoadFormatHOSTPORTValid(t *testing.T) {
	type cfg struct {
		BindAddress string `env:"SIMPLEENV_TEST_FORMAT_HOSTPORT;format=HOSTPORT"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_HOSTPORT", "127.0.0.1:8080")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid HOSTPORT format to pass, got %v", err)
	}
}

func TestLoadFormatMultipleValuesNotSupported(t *testing.T) {
	type cfg struct {
		Path string `env:"SIMPLEENV_TEST_FORMAT_MULTI;format=URL|FILE"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_MULTI", "http://localhost:8080")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected error for multiple format values, got nil")
	}
}

func TestLoadFormatUUIDValid(t *testing.T) {
	type cfg struct {
		RequestID string `env:"SIMPLEENV_TEST_FORMAT_UUID;format=UUID"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_UUID", "550e8400-e29b-41d4-a716-446655440000")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid UUID format to pass, got %v", err)
	}
}

func TestLoadFormatIPValid(t *testing.T) {
	type cfg struct {
		Address string `env:"SIMPLEENV_TEST_FORMAT_IP;format=IP"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_IP", "2001:db8::1")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid IP format to pass, got %v", err)
	}
}

func TestLoadFormatHEXValid(t *testing.T) {
	type cfg struct {
		Token string `env:"SIMPLEENV_TEST_FORMAT_HEX;format=HEX"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_HEX", "a1B2c3D4")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid HEX format to pass, got %v", err)
	}
}

func TestLoadFormatALPHANUMERICValid(t *testing.T) {
	type cfg struct {
		Code string `env:"SIMPLEENV_TEST_FORMAT_ALNUM;format=ALPHANUMERIC"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_ALNUM", "abc123XYZ")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid ALPHANUMERIC format to pass, got %v", err)
	}
}

func TestLoadFormatIDENTIFIERValid(t *testing.T) {
	type cfg struct {
		Name string `env:"SIMPLEENV_TEST_FORMAT_IDENTIFIER;format=IDENTIFIER"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_IDENTIFIER", "my-app_name_01")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected valid IDENTIFIER format to pass, got %v", err)
	}
}

func TestLoadFormatIDENTIFIERInvalid(t *testing.T) {
	type cfg struct {
		Name string `env:"SIMPLEENV_TEST_FORMAT_IDENTIFIER_BAD;format=IDENTIFIER"`
	}

	t.Setenv("SIMPLEENV_TEST_FORMAT_IDENTIFIER_BAD", "not valid")

	var c cfg
	err := Load(&c)
	if err == nil {
		t.Fatal("expected invalid IDENTIFIER format to fail, got nil")
	}
}

func TestLoadBoolTypeValid(t *testing.T) {
	type cfg struct {
		FeatureEnabled bool `env:"SIMPLEENV_TEST_BOOL"`
	}

	t.Setenv("SIMPLEENV_TEST_BOOL", "true")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected bool parsing to pass, got %v", err)
	}

	if !c.FeatureEnabled {
		t.Fatal("expected bool field to be true")
	}
}

func TestLoadInt64TypeValid(t *testing.T) {
	type cfg struct {
		MaxBytes int64 `env:"SIMPLEENV_TEST_INT64"`
	}

	t.Setenv("SIMPLEENV_TEST_INT64", "922337203685477580")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected int64 parsing to pass, got %v", err)
	}

	if c.MaxBytes != 922337203685477580 {
		t.Fatalf("expected parsed int64 value, got %d", c.MaxBytes)
	}
}

func TestLoadUintTypeValid(t *testing.T) {
	type cfg struct {
		WorkerCount uint `env:"SIMPLEENV_TEST_UINT"`
	}

	t.Setenv("SIMPLEENV_TEST_UINT", "12")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected uint parsing to pass, got %v", err)
	}

	if c.WorkerCount != 12 {
		t.Fatalf("expected parsed uint value 12, got %d", c.WorkerCount)
	}
}

func TestLoadDurationTypeValid(t *testing.T) {
	type cfg struct {
		Timeout time.Duration `env:"SIMPLEENV_TEST_DURATION"`
	}

	t.Setenv("SIMPLEENV_TEST_DURATION", "2m30s")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected duration parsing to pass, got %v", err)
	}

	if c.Timeout != 150*time.Second {
		t.Fatalf("expected parsed duration 150s, got %v", c.Timeout)
	}
}

func TestLoadTextUnmarshalerTypeValid(t *testing.T) {
	type cfg struct {
		Token customToken `env:"SIMPLEENV_TEST_TEXT_UNMARSHALER"`
	}

	t.Setenv("SIMPLEENV_TEST_TEXT_UNMARSHALER", "abc123")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected text unmarshaler parsing to pass, got %v", err)
	}

	if c.Token != "token:abc123" {
		t.Fatalf("expected custom token value, got %q", c.Token)
	}
}

func TestLoadTextUnmarshalerPointerTypeValid(t *testing.T) {
	type cfg struct {
		Token *customToken `env:"SIMPLEENV_TEST_TEXT_UNMARSHALER_PTR"`
	}

	t.Setenv("SIMPLEENV_TEST_TEXT_UNMARSHALER_PTR", "xyz789")

	var c cfg
	err := Load(&c)
	if err != nil {
		t.Fatalf("expected pointer text unmarshaler parsing to pass, got %v", err)
	}

	if c.Token == nil {
		t.Fatal("expected pointer token to be initialized")
	}

	if *c.Token != "token:xyz789" {
		t.Fatalf("expected pointer custom token value, got %q", *c.Token)
	}
}
