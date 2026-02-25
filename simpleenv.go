// Package simpleenv provides a simple way to configure your application
// environment variables using struct tags
package simpleenv

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type envTag struct {
	key      string
	options  []string
	optional bool
	hasTag   bool
}

func fieldConstraintError(fieldName, envKey, envValue, expected string) error {
	return fmt.Errorf("invalid value for field %q from ENV[%q]: got %q, expected %s", fieldName, envKey, envValue, expected)
}

// Load loads environment variables into the given struct
// and validates the constraints specified in the struct tags
//
//	e.g. `env:"ENVIRONMENT;oneof=development,test,staging,production"`
//	will load the environment variable `ENVIRONMENT` and validate
//	that it is one of the values in the `oneof` constraint.
//
//	valid constraints:
//	- optional: the environment variable does not need to exist and be set
//	- oneof: the environment variable must be one of the values in the `oneof` constraint list (separeted by commas)
//	- min: the environment variable must be greater than or equal to the value in the `min` constraint
//	- regex: the environment variable must match the regex pattern in the `regex` constraint
//	- format: the environment variable must match the format in the `format` constraint (only URL is supported)
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
// Make sure to pass a pointer to the struct, otherwise it will panic
// Load will return an error if the environment variables are not set
// (unless marked as optional) or if the value does not match the constraints
func Load(envConfig any) error {
	v := reflect.ValueOf(envConfig)
	if !v.IsValid() {
		return errors.New("failed to load env config, got nil value")
	}

	if v.Kind() == reflect.Struct {
		return errors.New("failed to load env config, expected pointer to struct (pass &cfg)")
	}

	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("failed to load env config, expected a non-nil pointer to an EnvConfig struct")
	}

	e := v.Elem()
	if e.Kind() != reflect.Struct {
		return errors.New("failed to load env config, expected a pointer to a struct")
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

		err = validateConstraints(fieldType, fieldTag.options, envValue)
		if err != nil {
			return err
		}

		parsedValue, err := parseValueFromEnv(fieldType, fieldTag.key, envValue)
		if err != nil {
			return err
		}

		err = assignFieldValue(fieldValue, parsedValue)
		if err != nil {
			return err
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

	return envTag{
		key:      envKey,
		options:  tagOptions,
		optional: optional,
		hasTag:   true,
	}, nil
}

func validateConstraints(fieldType reflect.StructField, tagOptions []string, envValue string) error {
	envKey := tagOptions[0]

	for _, constraint := range tagOptions[1:] {
		if constraint == "" || constraint == "optional" {
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
			format := strings.TrimPrefix(constraint, "format=")
			if !strings.EqualFold(format, "URL") {
				return fmt.Errorf("invalid tag for field %q (ENV[%q]): unsupported format %q", fieldType.Name, envKey, format)
			}
			if !isValidURL(envValue) {
				return fieldConstraintError(fieldType.Name, envKey, envValue, "a valid URL with http/https scheme")
			}
		default:
			return fmt.Errorf("invalid tag for field %q (ENV[%q]): unsupported constraint %q", fieldType.Name, envKey, constraint)
		}
	}

	return nil
}

func parseValueFromEnv(fieldType reflect.StructField, envKey, envValue string) (reflect.Value, error) {
	switch fieldType.Type.Kind() {
	case reflect.String:
		return reflect.ValueOf(envValue), nil

	case reflect.Int:
		intValue, err := strconv.Atoi(envValue)
		if err != nil {
			return reflect.Value{}, fieldConstraintError(fieldType.Name, envKey, envValue, "a valid int")
		}

		return reflect.ValueOf(intValue), nil
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

func assignFieldValue(field reflect.Value, val reflect.Value) error {
	if !field.IsValid() {
		return errors.New("failed to assign struct field value, field is not valid")
	}

	if !field.CanSet() {
		return errors.New("failed to assign struct field value, field can't be set")
	}

	if field.Type() != val.Type() {
		return errors.New("failed to assign struct field value, types don't match")
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
