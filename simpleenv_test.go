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

func strPtr(s string) *string {
	return &s
}

func loadSingleField(t *testing.T, fieldType reflect.Type, tagValue string, envValue *string) (reflect.Value, error) {
	t.Helper()

	key := strings.TrimSpace(strings.Split(tagValue, ";")[0])
	if envValue == nil {
		unsetEnv(t, key)
	} else {
		t.Setenv(key, *envValue)
	}

	cfgType := reflect.StructOf([]reflect.StructField{{
		Name: "Value",
		Type: fieldType,
		Tag:  reflect.StructTag(`env:"` + tagValue + `"`),
	}})

	loaded := reflect.New(cfgType)
	err := Load(loaded.Interface())
	return loaded.Elem().Field(0), err
}

func TestLoadInputValidation(t *testing.T) {
	x := 10
	tests := []struct {
		name     string
		input    any
		contains string
	}{
		{name: "nil input", input: nil, contains: "invalid Load input"},
		{name: "struct by value", input: struct{}{}, contains: "pass &cfg"},
		{name: "nil pointer", input: (*int)(nil), contains: "invalid Load input"},
		{name: "pointer to non-struct", input: &x, contains: "invalid Load input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Load(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("expected error to contain %q, got %q", tt.contains, err.Error())
			}
		})
	}
}

func TestLoadBehaviorCases(t *testing.T) {
	tests := []struct {
		name        string
		fieldType   reflect.Type
		tag         string
		envValue    *string
		wantErr     bool
		errContains []string
		wantValue   any
	}{
		{
			name:      "optional int missing keeps zero",
			fieldType: reflect.TypeOf(int(0)),
			tag:       "SIMPLEENV_TEST_OPTIONAL_INT;optional",
			envValue:  nil,
			wantValue: int(0),
		},
		{
			name:      "optional float missing keeps zero",
			fieldType: reflect.TypeOf(float64(0)),
			tag:       "SIMPLEENV_TEST_OPTIONAL_FLOAT;optional",
			envValue:  nil,
			wantValue: float64(0),
		},
		{
			name:      "optional with min missing skips validation",
			fieldType: reflect.TypeOf(int(0)),
			tag:       "SIMPLEENV_TEST_OPTIONAL_MIN;optional;min=1",
			envValue:  nil,
			wantValue: int(0),
		},
		{
			name:      "duration with min and max succeeds",
			fieldType: reflect.TypeOf(time.Duration(0)),
			tag:       "SIMPLEENV_TEST_DURATION_BOUNDED;min=1s;max=5s",
			envValue:  strPtr("2s"),
			wantValue: 2 * time.Second,
		},
		{
			name:        "duration below min returns error",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			tag:         "SIMPLEENV_TEST_DURATION_MIN_FAIL;min=3s",
			envValue:    strPtr("2s"),
			wantErr:     true,
			errContains: []string{"a value >= 3s"},
		},
		{
			name:        "duration above max returns error",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			tag:         "SIMPLEENV_TEST_DURATION_MAX_FAIL;max=3s",
			envValue:    strPtr("4s"),
			wantErr:     true,
			errContains: []string{"a value <= 3s"},
		},
		{
			name:        "duration with invalid min constraint returns error",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			tag:         "SIMPLEENV_TEST_DURATION_MIN_INVALID;min=abc",
			envValue:    strPtr("4s"),
			wantErr:     true,
			errContains: []string{"must be a valid duration"},
		},
		{
			name:        "duration min comparison with invalid value returns error",
			fieldType:   reflect.TypeOf(time.Duration(0)),
			tag:         "SIMPLEENV_TEST_DURATION_MIN_VALUE_INVALID;min=1s",
			envValue:    strPtr("nope"),
			wantErr:     true,
			errContains: []string{"valid duration for min comparison"},
		},
		{
			name:        "required missing returns error",
			fieldType:   reflect.TypeOf(int(0)),
			tag:         "SIMPLEENV_TEST_REQUIRED_MISSING;min=1",
			envValue:    nil,
			wantErr:     true,
			errContains: []string{"<unset>"},
		},
		{
			name:        "optional present invalid int returns error",
			fieldType:   reflect.TypeOf(int(0)),
			tag:         "SIMPLEENV_TEST_OPTIONAL_INVALID;optional",
			envValue:    strPtr("abc"),
			wantErr:     true,
			errContains: []string{"valid int"},
		},
		{
			name:        "optional present empty string returns error",
			fieldType:   reflect.TypeOf(""),
			tag:         "SIMPLEENV_TEST_OPTIONAL_EMPTY;optional",
			envValue:    strPtr(""),
			wantErr:     true,
			errContains: []string{"non-empty value"},
		},
		{
			name:        "required present empty string returns error",
			fieldType:   reflect.TypeOf(""),
			tag:         "SIMPLEENV_TEST_REQUIRED_EMPTY",
			envValue:    strPtr(""),
			wantErr:     true,
			errContains: []string{"non-empty value"},
		},
		{
			name:      "optional present empty string with allowempty succeeds",
			fieldType: reflect.TypeOf(""),
			tag:       "SIMPLEENV_TEST_OPTIONAL_EMPTY_ALLOWED;optional;allowempty",
			envValue:  strPtr(""),
			wantValue: "",
		},
		{
			name:      "required present empty string with allowempty succeeds",
			fieldType: reflect.TypeOf(""),
			tag:       "SIMPLEENV_TEST_REQUIRED_EMPTY_ALLOWED;allowempty",
			envValue:  strPtr(""),
			wantValue: "",
		},
		{
			name:      "whitespace in tag options is tolerated",
			fieldType: reflect.TypeOf(""),
			tag:       " SIMPLEENV_TEST_TAG_SPACES ; optional ; oneof=foo,bar ",
			envValue:  strPtr("foo"),
			wantValue: "foo",
		},
		{
			name:        "unknown constraint returns error",
			fieldType:   reflect.TypeOf(""),
			tag:         "SIMPLEENV_TEST_UNKNOWN_CONSTRAINT;nope=value",
			envValue:    strPtr("john"),
			wantErr:     true,
			errContains: []string{"unsupported constraint"},
		},
		{
			name:      "regex supports quoted pattern",
			fieldType: reflect.TypeOf(""),
			tag:       "SIMPLEENV_TEST_REGEX_QUOTED;regex='(http|https)://(localhost|127.0.0.1):[0-9]+'",
			envValue:  strPtr("http://localhost:8085"),
			wantValue: "http://localhost:8085",
		},
		{
			name:        "unknown format returns error",
			fieldType:   reflect.TypeOf(""),
			tag:         "SIMPLEENV_TEST_UNKNOWN_FORMAT;format=TOKEN;oneof=abc,def",
			envValue:    strPtr("abc"),
			wantErr:     true,
			errContains: []string{"unsupported format"},
		},
		{
			name:        "allowempty on int is invalid",
			fieldType:   reflect.TypeOf(int(0)),
			tag:         "SIMPLEENV_TEST_ALLOWEMPTY_INT;allowempty",
			envValue:    strPtr(""),
			wantErr:     true,
			errContains: []string{"allowempty is only supported"},
		},
		{
			name:        "error message includes field env and expected",
			fieldType:   reflect.TypeOf(int(0)),
			tag:         "SIMPLEENV_TEST_ERROR_SHAPE;min=1",
			envValue:    strPtr("abc"),
			wantErr:     true,
			errContains: []string{`field "Value"`, `ENV["SIMPLEENV_TEST_ERROR_SHAPE"]`, "expected"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := loadSingleField(t, tt.fieldType, tt.tag, tt.envValue)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, contains := range tt.errContains {
					if !strings.Contains(err.Error(), contains) {
						t.Fatalf("expected error to contain %q, got %q", contains, err.Error())
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !reflect.DeepEqual(value.Interface(), tt.wantValue) {
				t.Fatalf("unexpected value: got %#v, want %#v", value.Interface(), tt.wantValue)
			}
		})
	}
}

func TestLoadTagRules(t *testing.T) {
	t.Run("field without env tag is skipped", func(t *testing.T) {
		type cfg struct {
			Name         string `env:"SIMPLEENV_TEST_WITH_TAG"`
			DefaultValue string
		}

		t.Setenv("SIMPLEENV_TEST_WITH_TAG", "from-env")

		var c cfg
		err := Load(&c)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if c.Name != "from-env" {
			t.Fatalf("expected Name from env, got %q", c.Name)
		}
		if c.DefaultValue != "" {
			t.Fatalf("expected untagged field zero value, got %q", c.DefaultValue)
		}
	})

	t.Run("empty env tag value returns error", func(t *testing.T) {
		type cfg struct {
			NoKey string `env:""`
		}

		var c cfg
		err := Load(&c)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("malformed tag returns error", func(t *testing.T) {
		field := reflect.StructField{
			Name: "NoKey",
			Tag:  reflect.StructTag(`env:`),
		}

		_, err := parseEnvTag(field)
		if err == nil {
			t.Fatal("expected error for malformed env tag, got nil")
		}
	})
}

func TestLoadFormatCases(t *testing.T) {
	tests := []struct {
		name      string
		envKey    string
		format    string
		value     string
		wantError bool
		setup     func(t *testing.T) string
	}{
		{name: "URI valid", envKey: "SIMPLEENV_TEST_FORMAT_URI", format: "URI", value: "postgres://localhost:5432/mydb"},
		{
			name:   "FILE valid",
			envKey: "SIMPLEENV_TEST_FORMAT_FILE",
			format: "FILE",
			setup: func(t *testing.T) string {
				tmpFile, err := os.CreateTemp(t.TempDir(), "simpleenv-file-*.txt")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				_ = tmpFile.Close()
				return tmpFile.Name()
			},
		},
		{
			name:   "DIR valid",
			envKey: "SIMPLEENV_TEST_FORMAT_DIR",
			format: "DIR",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{name: "HOSTPORT valid", envKey: "SIMPLEENV_TEST_FORMAT_HOSTPORT", format: "HOSTPORT", value: "127.0.0.1:8080"},
		{name: "UUID valid", envKey: "SIMPLEENV_TEST_FORMAT_UUID", format: "UUID", value: "550e8400-e29b-41d4-a716-446655440000"},
		{name: "IP valid", envKey: "SIMPLEENV_TEST_FORMAT_IP", format: "IP", value: "2001:db8::1"},
		{name: "HEX valid", envKey: "SIMPLEENV_TEST_FORMAT_HEX", format: "HEX", value: "a1B2c3D4"},
		{name: "ALPHANUMERIC valid", envKey: "SIMPLEENV_TEST_FORMAT_ALNUM", format: "ALPHANUMERIC", value: "abc123XYZ"},
		{name: "IDENTIFIER valid", envKey: "SIMPLEENV_TEST_FORMAT_IDENTIFIER", format: "IDENTIFIER", value: "my-app_name_01"},
		{name: "IDENTIFIER invalid", envKey: "SIMPLEENV_TEST_FORMAT_IDENTIFIER_BAD", format: "IDENTIFIER", value: "not valid", wantError: true},
		{name: "multiple formats unsupported", envKey: "SIMPLEENV_TEST_FORMAT_MULTI", format: "URL|FILE", value: "http://localhost:8080", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.value
			if tt.setup != nil {
				value = tt.setup(t)
			}

			tag := tt.envKey + ";format=" + tt.format
			_, err := loadSingleField(t, reflect.TypeOf(""), tag, strPtr(value))
			if tt.wantError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestLoadTypeCases(t *testing.T) {
	tests := []struct {
		name        string
		fieldType   reflect.Type
		envKey      string
		tag         string
		envValue    string
		wantValue   any
		wantPointer bool
	}{
		{name: "bool", fieldType: reflect.TypeOf(true), envKey: "SIMPLEENV_TEST_BOOL", envValue: "true", wantValue: true},
		{name: "int64", fieldType: reflect.TypeOf(int64(0)), envKey: "SIMPLEENV_TEST_INT64", envValue: "922337203685477580", wantValue: int64(922337203685477580)},
		{name: "uint", fieldType: reflect.TypeOf(uint(0)), envKey: "SIMPLEENV_TEST_UINT", envValue: "12", wantValue: uint(12)},
		{name: "duration", fieldType: reflect.TypeOf(time.Duration(0)), envKey: "SIMPLEENV_TEST_DURATION", envValue: "2m30s", wantValue: 150 * time.Second},
		{name: "text unmarshaler value", fieldType: reflect.TypeOf(customToken("")), envKey: "SIMPLEENV_TEST_TEXT_UNMARSHALER", envValue: "abc123", wantValue: customToken("token:abc123")},
		{name: "text unmarshaler pointer", fieldType: reflect.TypeOf((*customToken)(nil)), envKey: "SIMPLEENV_TEST_TEXT_UNMARSHALER_PTR", envValue: "xyz789", wantValue: customToken("token:xyz789"), wantPointer: true},
		{name: "text unmarshaler allowempty", fieldType: reflect.TypeOf(customToken("")), envKey: "SIMPLEENV_TEST_TEXT_UNMARSHALER_EMPTY", tag: "SIMPLEENV_TEST_TEXT_UNMARSHALER_EMPTY;allowempty", envValue: "", wantValue: customToken("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagValue := tt.envKey
			if tt.tag != "" {
				tagValue = tt.tag
			}

			value, err := loadSingleField(t, tt.fieldType, tagValue, strPtr(tt.envValue))
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if tt.wantPointer {
				if value.IsNil() {
					t.Fatal("expected pointer value, got nil")
				}
				if !reflect.DeepEqual(value.Elem().Interface(), tt.wantValue) {
					t.Fatalf("unexpected pointer value: got %#v, want %#v", value.Elem().Interface(), tt.wantValue)
				}
				return
			}

			if !reflect.DeepEqual(value.Interface(), tt.wantValue) {
				t.Fatalf("unexpected value: got %#v, want %#v", value.Interface(), tt.wantValue)
			}
		})
	}
}
