package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/jeremywohl/flatten"
	"github.com/mcuadros/go-defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type Loader struct {
	v *viper.Viper
}

type LoaderOption func(*Loader)

type SecretString string

func (s SecretString) String() string {
	return "****************"
}

func (s SecretString) Secret() string {
	return string(s)
}

// WithViper sets the given viper instance for loading configs
// instead of the default configured one
func WithViper(in *viper.Viper) LoaderOption {
	return func(l *Loader) {
		l.v = in
	}
}

// WithName sets the file name of the config file without
// the extension
func WithName(in string) LoaderOption {
	return func(l *Loader) {
		l.v.SetConfigName(in)
	}
}

// WithPath adds config path to search the config file in,
// can be used multiple times to add multiple paths to search
func WithPath(in string) LoaderOption {
	return func(l *Loader) {
		l.v.AddConfigPath(in)
	}
}

// WithType sets the type of the configuration e.g. "json",
// "yaml", "hcl"
// Also used for the extension of the file
func WithType(in string) LoaderOption {
	return func(l *Loader) {
		l.v.SetConfigType(in)
	}
}

// WithEnvPrefix sets the prefix for keys when checking for configs
// in environment variables. Internally concatenates with keys
// with `_` in between
func WithEnvPrefix(in string) LoaderOption {
	return func(l *Loader) {
		l.v.SetEnvPrefix(in)
	}
}

// WithEnvKeyReplacer sets the `old` string to be replaced with
// the `new` string environmental variable to a key that does
// not match it.
func WithEnvKeyReplacer(old string, new string) LoaderOption {
	return func(l *Loader) {
		l.v.SetEnvKeyReplacer(strings.NewReplacer(old, new))
	}
}

// NewLoader returns a config loader with given LoaderOption(s)
func NewLoader(options ...LoaderOption) *Loader {
	loader := &Loader{
		v: getViperWithDefaults(),
	}

	for _, option := range options {
		option(loader)
	}
	return loader
}

// Load loads configuration into the given mapstructure (https://github.com/mitchellh/mapstructure)
// from a config.yaml file and overrides with any values set in env variables
func (l *Loader) Load(config interface{}) error {
	if err := verifyParamIsPtrToStructElsePanic(config); err != nil {
		return err
	}

	l.v.AutomaticEnv()

	if err := l.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("unable to read configs using viper: %v", err)
		}
	}

	configKeys, err := getFlattenedStructKeys(config)
	if err != nil {
		return fmt.Errorf("unable to get all config keys from struct: %v", err)
	}

	// Bind each conf fields from struct to environment vars
	for key := range configKeys {
		if err := l.v.BindEnv(configKeys[key]); err != nil {
			return fmt.Errorf("unable to bind env keys: %v", err)
		}
	}

	// set defaults using the default struct tag
	defaults.SetDefaults(config)

	if err := l.v.Unmarshal(config); err != nil {
		return fmt.Errorf("unable to load config to struct: %v", err)
	}
	return nil
}

func verifyParamIsPtrToStructElsePanic(param interface{}) error {
	value := reflect.ValueOf(param)
	if value.Kind() != reflect.Ptr {
		return fmt.Errorf("require ptr to a struct for Load. Got %v", value.Kind())
	} else {
		value = reflect.Indirect(value)
		if value.Kind() != reflect.Struct {
			return fmt.Errorf("require ptr to a struct for Load. got ptr to %v", value.Kind())
		}
	}
	return nil
}

func getViperWithDefaults() *viper.Viper {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return v
}

func getFlattenedStructKeys(config interface{}) ([]string, error) {
	var structMap map[string]interface{}
	if err := mapstructure.Decode(config, &structMap); err != nil {
		return nil, err
	}

	flat, err := flatten.Flatten(structMap, "", flatten.DotStyle)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}

	return keys, nil
}

func GetPrintable(config interface{}) ([]byte, error) {
	var structMap map[string]interface{}

	newConfig := reflect.New(reflect.ValueOf(config).Elem().Type()).Interface()

	// Copy data to map so we can iterate on each field using DecodeHook
	if err := Decode(config, &structMap); err != nil {
		return nil, err
	}

	// Copy from map to newConfig to run DecodeHook on each field
	if err := Decode(structMap, &newConfig); err != nil {
		return nil, err
	}

	// Copy back to map to get printable field names which are
	// generated after considering the mapstructure struct tags if any
	if err := Decode(newConfig, &structMap); err != nil {
		return nil, err
	}

	printable, err := json.MarshalIndent(structMap, "", "  ")
	return printable, err
}

func Decode(input interface{}, output interface{}) error {
	// Config same as what viper uses with additional
	// SecretStringMaskHookFunc() DecodeHook added
	config := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			SecretStringMaskHookFunc(),
		),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func SecretStringMaskHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		var secret SecretString

		if f != reflect.TypeOf(secret) {
			return data, nil
		}

		if t != reflect.TypeOf(secret) {
			return data, nil
		}
		return data.(SecretString).String(), nil
	}
}
