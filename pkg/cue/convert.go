package cue

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/apis/types"
)

// para struct contains the parameter
const specValue = "parameter"

// GetParameters get parameter from cue template
func GetParameters(templatePath string) ([]types.Parameter, error) {
	r := cue.Runtime{}
	b, err := ioutil.ReadFile(filepath.Clean(templatePath))
	if err != nil {
		return nil, err
	}
	template, err := r.Compile("", string(b)+BaseTemplate)
	if err != nil {
		return nil, err
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return nil, err
	}
	// find the parameter definition
	var paraDef cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		paraDef = tempStruct.Field(i)
		if paraDef.Name == specValue {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("arguments not exist")
	}
	arguments, err := paraDef.Value.Struct()
	if err != nil {
		return nil, fmt.Errorf("arguments not defined as struct %w", err)
	}
	// parse each fields in the parameter fields
	var params []types.Parameter
	for i := 0; i < arguments.Len(); i++ {
		fi := arguments.Field(i)
		if fi.IsDefinition {
			continue
		}
		var param = types.Parameter{

			Name:     fi.Name,
			Required: !fi.IsOptional,
		}
		val := fi.Value
		param.Type = fi.Value.IncompleteKind()
		if def, ok := val.Default(); ok && def.IsConcrete() {
			param.Required = false
			param.Type = def.Kind()
			param.Default = GetDefault(def)
		}
		if param.Default == nil {
			param.Default = getDefaultByKind(param.Type)
		}
		param.Short, param.Usage, param.Alias = RetrieveComments(val)

		params = append(params, param)
	}
	return params, nil
}

func getDefaultByKind(k cue.Kind) interface{} {
	// nolint:exhaustive
	switch k {
	case cue.IntKind:
		var d int64
		return d
	case cue.StringKind:
		var d string
		return d
	case cue.BoolKind:
		var d bool
		return d
	case cue.NumberKind, cue.FloatKind:
		var d float64
		return d
	default:
		// assume other cue kind won't be valid parameter
	}
	return nil
}

// GetDefault evaluate default Go value from CUE
func GetDefault(val cue.Value) interface{} {
	// nolint:exhaustive
	switch val.Kind() {
	case cue.IntKind:
		if d, err := val.Int64(); err == nil {
			return d
		}
	case cue.StringKind:
		if d, err := val.String(); err == nil {
			return d
		}
	case cue.BoolKind:
		if d, err := val.Bool(); err == nil {
			return d
		}
	case cue.NumberKind, cue.FloatKind:
		if d, err := val.Float64(); err == nil {
			return d
		}
	default:
	}
	return getDefaultByKind(val.Kind())
}

const (
	// UsagePrefix defines the usage display for KubeVela CLI
	UsagePrefix = "+usage="
	// ShortPrefix defines the short argument for KubeVela CLI
	ShortPrefix = "+short="
	// AliasPrefix is an alias of the name of a parameter element, in order to making it more friendly to Cli users
	AliasPrefix = "+alias="
)

// RetrieveComments will retrieve Usage, Short and Alias from CUE Value
func RetrieveComments(value cue.Value) (string, string, string) {
	var short, usage, alias string
	docs := value.Doc()
	for _, doc := range docs {
		lines := strings.Split(doc.Text(), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "//")
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, ShortPrefix) {
				short = strings.TrimPrefix(line, ShortPrefix)
			}
			if strings.HasPrefix(line, UsagePrefix) {
				usage = strings.TrimPrefix(line, UsagePrefix)
			}
			if strings.HasPrefix(line, AliasPrefix) {
				alias = strings.TrimPrefix(line, AliasPrefix)
			}
		}
	}
	return short, usage, alias
}
