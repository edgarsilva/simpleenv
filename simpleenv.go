// Package simpleenv provides a simple way to configure your application
// environment variables using struct tags
package simpleenv

import (
	"encoding"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type envTag struct {
	key        string
	options    []string
	optional   bool
	allowEmpty bool
	trimSpace  bool
	hasTag     bool
}

var (
	timeDurationType    = reflect.TypeOf(time.Duration(0))
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

func fieldConstraintError(fieldName, envKey, envValue, expected string) error {
	return fmt.Errorf("invalid value for field %q from ENV[%q]: got %q, expected %s", fieldName, envKey, envValue, expected)
}

func loadInputError(expected string) error {
	return fmt.Errorf("invalid Load input: expected %s", expected)
}

// Load loads environment variables into the given struct
// and validates the constraints specified in the struct tags
//
//	e.g. `env:"ENVIRONMENT;oneof=development,test,staging,production"`
//	will load the environment variable `ENVIRONMENT` and validate
//	that it is one of the values in the `oneof` constraint.
//
//	valid constraints:
//	- optional: the environment variable may be missing
//	- allowempty: only for string or text unmarshaler fields; allows KEY="" when present
//	- trimspace: only for string or text unmarshaler fields; trims leading/trailing whitespace before validation/parsing
//	- oneof: the environment variable must be one of the values in the `oneof` constraint list (separeted by commas)
//	- min: the environment variable must be greater than or equal to the value in the `min` constraint
//	- max: the environment variable must be less than or equal to the value in the `max` constraint
//	- regex: the environment variable must match the regex pattern in the `regex` constraint
//	- format: the environment variable must match the format in the `format` constraint
//	  supported formats: URL, URI, FILE, DIR, HOSTPORT, UUID, IP, HEX, ALPHANUMERIC, IDENTIFIER
//	  note: only one format value is supported (e.g. `format=URL`)
//
//	supported field types:
//	- string
//	- bool
//	- int
//	- int64
//	- uint
//	- float64
//	- time.Duration
//	- custom types implementing encoding.TextUnmarshaler
//
//	example:
//		type AppEnv struct {
//			Environment string `env:"ENVIRONMENT;oneof=development,test,staging,production"`
//			Version     float64 `env:"VERSION;"`
//			ApiURL      string  `env:"API_URL;;format=URL"`
//			Concurrency int     `env:"CONCURRENCY;optional;min=1"`
//		}
//
//		appEnv := AppEnv{}
//		appenv.Load(&appEnv)
//
//		appEnv.Environment // "development"
//
// Make sure to pass a non-nil pointer to a struct (for example: &cfg),
// otherwise Load returns an input validation error.
// Load also returns an error if required environment variables are not set
// or if any value does not match the constraints.
func Load(envConfig any) error {
	v := reflect.ValueOf(envConfig)
	if !v.IsValid() {
		return loadInputError("a non-nil pointer to a struct")
	}

	if v.Kind() == reflect.Struct {
		return loadInputError("a non-nil pointer to a struct (pass &cfg so it can be updated)")
	}

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return loadInputError("a non-nil pointer to a struct")
	}

	e := v.Elem()
	if e.Kind() != reflect.Struct {
		return loadInputError("a non-nil pointer to a struct")
	}

	t := e.Type()

	for i := range t.NumField() {
		fieldType := t.Field(i)
		fieldValue := e.Field(i)

		fieldTag, err := parseEnvTag(fieldType)
		if err != nil {
			return err
		}
		if !fieldTag.hasTag {
			continue
		}

		envValue, found := os.LookupEnv(fieldTag.key)
		if !found {
			if fieldTag.optional {
				continue
			}

			return fieldConstraintError(fieldType.Name, fieldTag.key, "<unset>", "a value to set or to be marked as optional")
		}

		normalizedValue := envValue
		if fieldTag.trimSpace {
			normalizedValue = strings.TrimSpace(envValue)
		}

		if normalizedValue == "" && !fieldTag.allowEmpty {
			return fieldConstraintError(fieldType.Name, fieldTag.key, normalizedValue, "a non-empty value")
		}

		err = validateConstraints(fieldType, fieldTag.options, normalizedValue)
		if err != nil {
			return err
		}

		parsedValue, err := parseValueFromEnv(fieldType, fieldTag.key, normalizedValue)
		if err != nil {
			return err
		}

		err = assignFieldValue(fieldValue, parsedValue)
		if err != nil {
			return fmt.Errorf("failed to assign field %q from ENV[%q]: %w", fieldType.Name, fieldTag.key, err)
		}
	}

	return nil
}

func parseEnvTag(fieldType reflect.StructField) (envTag, error) {
	tagValue, hasEnvTag := fieldType.Tag.Lookup("env")
	if !hasEnvTag {
		if strings.Contains(string(fieldType.Tag), "env:") {
			return envTag{}, fmt.Errorf("invalid tag for field %q: malformed env tag, expected env:\"ENV_KEY;...\"", fieldType.Name)
		}

		return envTag{hasTag: false}, nil
	}

	if strings.TrimSpace(tagValue) == "" {
		return envTag{}, fmt.Errorf("invalid tag for field %q: env key cannot be empty", fieldType.Name)
	}

	rawTagOptions := strings.Split(tagValue, ";")
	tagOptions := make([]string, 0, len(rawTagOptions))
	for _, option := range rawTagOptions {
		tagOptions = append(tagOptions, strings.TrimSpace(option))
	}

	if len(tagOptions) < 1 || strings.TrimSpace(tagOptions[0]) == "" {
		return envTag{}, fmt.Errorf("invalid tag for field %q: env key cannot be empty", fieldType.Name)
	}

	envKey := strings.TrimSpace(tagOptions[0])
	optional := slices.Contains(tagOptions, "optional")
	allowEmpty := slices.Contains(tagOptions, "allowempty")
	trimSpace := slices.Contains(tagOptions, "trimspace")
	if allowEmpty && !supportsAllowEmpty(fieldType.Type) {
		return envTag{}, fmt.Errorf("invalid tag for field %q (ENV[%q]): allowempty is only supported for string or encoding.TextUnmarshaler types", fieldType.Name, envKey)
	}

	if trimSpace && !supportsTrimSpace(fieldType.Type) {
		return envTag{}, fmt.Errorf("invalid tag for field %q (ENV[%q]): trimspace is only supported for string or encoding.TextUnmarshaler types", fieldType.Name, envKey)
	}

	return envTag{
		key:        envKey,
		options:    tagOptions,
		optional:   optional,
		allowEmpty: allowEmpty,
		trimSpace:  trimSpace,
		hasTag:     true,
	}, nil
}

func validateConstraints(fieldType reflect.StructField, tagOptions []string, envValue string) error {
	envKey := tagOptions[0]

	for _, constraint := range tagOptions[1:] {
		if constraint == "" || constraint == "optional" || constraint == "allowempty" || constraint == "trimspace" {
			continue
		}

		switch {
		case strings.HasPrefix(constraint, "oneof="):
			strOpts := strings.TrimPrefix(constraint, "oneof=")
			opts := strings.Split(strOpts, ",")
			if !slices.Contains(opts, envValue) {
				return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("one of [%s]", strOpts))
			}
		case strings.HasPrefix(constraint, "min="):
			minstr := strings.TrimPrefix(constraint, "min=")

			if fieldType.Type == timeDurationType {
				minDuration, err := time.ParseDuration(minstr)
				if err != nil {
					return fmt.Errorf("invalid tag for field %q (ENV[%q]): %q must be a valid duration", fieldType.Name, envKey, constraint)
				}

				fieldDuration, err := time.ParseDuration(envValue)
				if err != nil {
					return fieldConstraintError(fieldType.Name, envKey, envValue, "a valid duration for min comparison")
				}

				if fieldDuration < minDuration {
					return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("a value >= %s", minstr))
				}

				continue
			}

			min, err := strconv.ParseFloat(minstr, 64)
			if err != nil {
				return fmt.Errorf("invalid tag for field %q (ENV[%q]): %q must be a valid number", fieldType.Name, envKey, constraint)
			}

			fieldValue, err := strconv.ParseFloat(envValue, 64)
			if err != nil {
				return fieldConstraintError(fieldType.Name, envKey, envValue, "a numeric value for min comparison")
			}

			if fieldValue < min {
				return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("a value >= %s", minstr))
			}
		case strings.HasPrefix(constraint, "max="):
			maxstr := strings.TrimPrefix(constraint, "max=")

			if fieldType.Type == timeDurationType {
				maxDuration, err := time.ParseDuration(maxstr)
				if err != nil {
					return fmt.Errorf("invalid tag for field %q (ENV[%q]): %q must be a valid duration", fieldType.Name, envKey, constraint)
				}

				fieldDuration, err := time.ParseDuration(envValue)
				if err != nil {
					return fieldConstraintError(fieldType.Name, envKey, envValue, "a valid duration for max comparison")
				}

				if fieldDuration > maxDuration {
					return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("a value <= %s", maxstr))
				}

				continue
			}

			max, err := strconv.ParseFloat(maxstr, 64)
			if err != nil {
				return fmt.Errorf("invalid tag for field %q (ENV[%q]): %q must be a valid number", fieldType.Name, envKey, constraint)
			}

			fieldValue, err := strconv.ParseFloat(envValue, 64)
			if err != nil {
				return fieldConstraintError(fieldType.Name, envKey, envValue, "a numeric value for max comparison")
			}

			if fieldValue > max {
				return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("a value <= %s", maxstr))
			}
		case strings.HasPrefix(constraint, "regex="):
			patternstr := normalizeQuotedValue(strings.TrimPrefix(constraint, "regex="))
			_, err := matchRegex(patternstr, envValue)
			if err != nil {
				return fieldConstraintError(fieldType.Name, envKey, envValue, fmt.Sprintf("to match regex %q", patternstr))
			}
		case strings.HasPrefix(constraint, "format="):
			format := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(constraint, "format=")))
			if strings.Contains(format, "|") {
				return fmt.Errorf("invalid tag for field %q (ENV[%q]): multiple format values are not supported, got %q", fieldType.Name, envKey, format)
			}

			expected, ok := validateFormat(format, envValue)
			if expected == "" {
				return fmt.Errorf("invalid tag for field %q (ENV[%q]): unsupported format %q", fieldType.Name, envKey, format)
			}
			if !ok {
				return fieldConstraintError(fieldType.Name, envKey, envValue, expected)
			}
		default:
			return fmt.Errorf("invalid tag for field %q (ENV[%q]): unsupported constraint %q", fieldType.Name, envKey, constraint)
		}
	}

	return nil
}

func parseValueFromEnv(fieldType reflect.StructField, envKey, envValue string) (reflect.Value, error) {
	if fieldType.Type == timeDurationType {
		durationValue, err := time.ParseDuration(envValue)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid time.Duration (for example: 500ms, 2s, 1m)")
		}

		return reflect.ValueOf(durationValue), nil
	}

	if unmarshaledValue, ok, err := parseWithTextUnmarshaler(fieldType, envKey, envValue); ok || err != nil {
		return unmarshaledValue, err
	}

	switch fieldType.Type.Kind() {
	case reflect.String:
		return reflect.ValueOf(envValue), nil
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(envValue)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid bool")
		}

		return reflect.ValueOf(boolValue), nil

	case reflect.Int:
		intValue, err := strconv.Atoi(envValue)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid int")
		}

		return reflect.ValueOf(intValue), nil
	case reflect.Int64:
		int64Value, err := strconv.ParseInt(envValue, 10, 64)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid int64")
		}

		return reflect.ValueOf(int64Value), nil
	case reflect.Uint:
		uintValue, err := strconv.ParseUint(envValue, 10, strconv.IntSize)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid uint")
		}

		return reflect.ValueOf(uint(uintValue)), nil
	case reflect.Float64:
		floatValue, err := strconv.ParseFloat(envValue, 64)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid float64")
		}

		return reflect.ValueOf(floatValue), nil
	default:
		return reflect.Value{}, fmt.Errorf("unsupported type for field %q (ENV[%q]): %v", fieldType.Name, envKey, fieldType.Type)
	}
}

func parseWithTextUnmarshaler(fieldType reflect.StructField, envKey, envValue string) (reflect.Value, bool, error) {
	isPointerField := fieldType.Type.Kind() == reflect.Pointer
	valuePtr := castToValuePtr(fieldType)

	if !valuePtr.Type().Implements(textUnmarshalerType) {
		return reflect.Value{}, false, nil
	}

	unmarshaler := valuePtr.Interface().(encoding.TextUnmarshaler)
	if err := unmarshaler.UnmarshalText([]byte(envValue)); err != nil {
		return reflect.Value{}, true, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid value for custom text unmarshaler")
	}

	if isPointerField {
		return valuePtr, true, nil
	}

	return valuePtr.Elem(), true, nil
}

func supportsAllowEmpty(fieldType reflect.Type) bool {
	if fieldType.Kind() == reflect.String {
		return true
	}

	if fieldType.Kind() == reflect.Pointer {
		return fieldType.Implements(textUnmarshalerType)
	}

	return reflect.PointerTo(fieldType).Implements(textUnmarshalerType)
}

func supportsTrimSpace(fieldType reflect.Type) bool {
	return supportsAllowEmpty(fieldType)
}

func castToValuePtr(fieldType reflect.StructField) reflect.Value {
	var valuePtr reflect.Value
	if fieldType.Type.Kind() == reflect.Pointer {
		valuePtr = reflect.New(fieldType.Type.Elem())
	} else {
		valuePtr = reflect.New(fieldType.Type)
	}

	return valuePtr
}

func assignFieldValue(field reflect.Value, val reflect.Value) error {
	if !field.IsValid() {
		return errors.New("field is not valid")
	}

	if !field.CanSet() {
		return errors.New("field is not settable")
	}

	if field.Type() != val.Type() {
		return fmt.Errorf("type mismatch: field type %v != parsed type %v", field.Type(), val.Type())
	}

	field.Set(val)
	return nil
}

func matchRegex(pattern, str string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("failed to compile env config regex '%v': %v", pattern, err)
	}

	matches := re.MatchString(str)
	if !matches {
		return false, errors.New("env var value does not match given pattern")
	}
	return true, nil
}

func normalizeQuotedValue(s string) string {
	if len(s) < 2 {
		return s
	}

	if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}

	return s
}

func isValidURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	allowedSchemes := []string{"http", "https"}
	for _, scheme := range allowedSchemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return true
		}
	}

	return false
}

func validateFormat(format, value string) (expected string, ok bool) {
	switch format {
	case "URL":
		return "a valid URL with http/https scheme", isValidURL(value)
	case "URI":
		return "a valid URI with scheme", isValidURI(value)
	case "FILE":
		return "an existing file path", isExistingFile(value)
	case "DIR":
		return "an existing directory path", isExistingDir(value)
	case "HOSTPORT":
		return "a valid host:port value", isValidHostPort(value)
	case "UUID":
		return "a valid UUID", isValidUUID(value)
	case "IP":
		return "a valid IPv4 or IPv6 address", isValidIP(value)
	case "HEX":
		return "a valid hexadecimal value", isHex(value)
	case "ALPHANUMERIC":
		return "a value containing only letters and numbers", isAlphanumeric(value)
	case "IDENTIFIER":
		return "a value containing only letters, numbers, underscores, or hyphens", isIdentifier(value)
	default:
		return "", false
	}
}

func isValidURI(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	return u.Scheme != ""
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func isExistingDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

func isValidHostPort(value string) bool {
	_, _, err := net.SplitHostPort(value)
	return err == nil
}

func isValidUUID(value string) bool {
	match, _ := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`, value)
	return match
}

func isValidIP(value string) bool {
	return net.ParseIP(value) != nil
}

func isHex(value string) bool {
	match, _ := regexp.MatchString(`^[0-9a-fA-F]+$`, value)
	return match
}

func isAlphanumeric(value string) bool {
	match, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, value)
	return match
}

func isIdentifier(value string) bool {
	match, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, value)
	return match
}
