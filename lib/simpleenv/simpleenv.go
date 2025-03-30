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
	fmt.Println("🚀 Loading env vars...")
	v := reflect.ValueOf(envConfig)
	e := v.Elem()
	t := e.Type()

	fmt.Printf("🔎 Scanning env vars...")
	for i := range t.NumField() {
		fieldType := t.Field(i)
		fieldValue := e.Field(i)

		envValue, err := parseValueFromEnv(fieldType)
		if err != nil {
			return err
		}

		err = validateConstraints(fieldType)
		if err != nil {
			return err
		}

		_, err = assignFieldValue(fieldValue, envValue)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateConstraints(fieldType reflect.StructField) error {
	tag := fieldType.Tag.Get("env")
	tagOptions := strings.Split(tag, ";")
	if len(tagOptions) < 1 {
		return errors.New("failed to find env var name, missing struct tag? e.g. `env:\"environment\"`")
	}

	envKey := tagOptions[0]
	envValue := os.Getenv(envKey)

	for _, constraint := range tagOptions {
		if envValue == "" && !slices.Contains(tagOptions, "optional") {
			return fmt.Errorf("failed to find value for ENV[\"%v\"], which is required in the AppEnv struct field '%v'", envKey, fieldType.Name)
		}
		switch {
		case strings.HasPrefix(constraint, "oneof="):
			strOpts := strings.TrimPrefix(constraint, "oneof=")
			opts := strings.Split(strOpts, ",")
			if !slices.Contains(opts, envValue) {
				return fmt.Errorf("failed to match env var %v with value '%v', must be one of [%v]", fieldType.Name, envValue, strOpts)
			}
		case strings.HasPrefix(constraint, "min="):
			minstr := strings.TrimPrefix(constraint, "min=")
			min, err := strconv.ParseFloat(minstr, 64)
			if err != nil {
				return fmt.Errorf("failed to parse min value for %v, in struct tag min=", fieldType.Name)
			}

			fieldValue, err := strconv.ParseFloat(envValue, 64)
			if err != nil {
				return fmt.Errorf("failed to parse float value in env var %v, for struct tag constraint %v", envKey, fieldType.Name)
			}

			if fieldValue < min {
				return fmt.Errorf("failed min value constraint for envvar[%v] in struct field %v, %v", envKey, fieldType.Name, constraint)
			}
		case strings.HasPrefix(constraint, "max="):
			maxstr := strings.TrimPrefix(constraint, "max=")
			max, err := strconv.ParseFloat(maxstr, 64)
			if err != nil {
				return fmt.Errorf("failed to parse max value for %v, in struct tag max=", fieldType.Name)
			}

			fieldValue, err := strconv.ParseFloat(envValue, 64)
			if err != nil {
				return fmt.Errorf("failed to parse float value in envvar[%v], for struct tag constraint %v", envKey, fieldType.Name)
			}

			if fieldValue > max {
				return fmt.Errorf("failed max value constraint for envvar[%v] in struct field %v, %v", envKey, fieldType.Name, constraint)
			}
		case strings.HasPrefix(constraint, "regex="):
			patternstr := strings.TrimPrefix(constraint, "regex=")
			_, err := matchRegex(patternstr, envValue)
			if err != nil {
				return fmt.Errorf("failed regex match for env var %v, with regex constraint in struct tag %v", envKey, fieldType.Name)
			}
		case strings.HasPrefix(constraint, "format="):
			format := strings.TrimPrefix(constraint, "format=")
			if format != "URL" {
				return nil
			}
			if !isValidURL(envValue) {
				return fmt.Errorf("failed URL format for env var %v, with regex constraint in struct tag %v", envKey, fieldType.Name)
			}
		default:
		}
	}

	return nil
}

func parseValueFromEnv(fieldType reflect.StructField) (reflect.Value, error) {
	tag := fieldType.Tag.Get("env")
	tagOptions := strings.Split(tag, ";")
	if len(tagOptions) < 1 {
		return reflect.Value{}, errors.New("failed to find env var name, missing struct tag env? e.g. `env:\"environment\"`")
	}

	envKey := tagOptions[0]
	envValue := os.Getenv(envKey)

	switch fieldType.Type.Kind() {
	case reflect.String:
		return reflect.ValueOf(envValue), nil

	case reflect.Int:
		intValue, err := strconv.Atoi(envValue)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("failed to cast env variable '%v' value to int, struct field '%v: type %v'", envKey, fieldType.Name, fieldType.Type)
		}

		return reflect.ValueOf(intValue), nil
	case reflect.Float64:
		floatValue, err := strconv.ParseFloat(envValue, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("failed to parse env variable '%v' value to float64, struct field '%v: type %v'", envKey, fieldType.Name, fieldType.Type)
		}

		return reflect.ValueOf(floatValue), nil
	default:
		return reflect.Value{}, fmt.Errorf("failed to parse env variable '%v' into struct field '%v: type %v'", envKey, fieldType.Name, fieldType.Type)
	}
}

func assignFieldValue(field reflect.Value, val reflect.Value) (reflect.Value, error) {
	if !field.IsValid() {
		return field, errors.New("failed to assign struct field value, field is not valid")
	}

	if !field.CanSet() {
		return field, errors.New("failed to assign struct field value, field can't be set")
	}

	if field.Type() != val.Type() {
		return field, errors.New("failed to assign struct field value, types don't match")
	}

	field.Set(val)
	return field, nil
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
