/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package process

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

const (
	// OutputFieldName is the reference of context base object
	OutputFieldName = "output"
	// OutputsFieldName is the reference of context Auxiliaries
	OutputsFieldName = "outputs"
	// ConfigFieldName is the reference of context config
	ConfigFieldName = "config"
	// ContextName is the name of context
	ContextName = "name"
	// ContextAppName is the appName of context
	ContextAppName = "appName"
	// ContextAppRevision is the revision name of app of context
	ContextAppRevision = "appRevision"
	// ContextAppRevisionNum is the revision num of app of context
	ContextAppRevisionNum = "appRevisionNum"
	// ContextNamespace is the namespace of the app
	ContextNamespace = "namespace"
	// OutputSecretName is used to store all secret names which are generated by cloud resource components
	OutputSecretName = "outputSecretName"
)

// Context defines Rendering Context Interface
type Context interface {
	SetBase(base model.Instance)
	AppendAuxiliaries(auxiliaries ...Auxiliary)
	Output() (model.Instance, []Auxiliary)
	BaseContextFile() string
	ExtendedContextFile() string
	BaseContextLabels() map[string]string
	SetConfigs(configs []map[string]string)
	InsertSecrets(outputSecretName string, requiredSecrets []RequiredSecrets)
}

// Auxiliary are objects rendered by definition template.
// the format for auxiliary resource is always: `outputs.<resourceName>`, it can be auxiliary workload or trait
type Auxiliary struct {
	Ins model.Instance
	// Type will be used to mark definition label for OAM runtime to get the CRD
	// It's now required for trait and main workload object. Extra workload CR object will not have the type.
	Type string

	// Workload or trait with multiple `outputs` will have a name, if name is empty, than it's the main of this type.
	Name string
}

type templateContext struct {
	// name is the component name of Application
	name string
	// appName is the name of Application
	appName string
	// appRevision is the revision name of Application
	appRevision string
	configs     []map[string]string
	base        model.Instance
	auxiliaries []Auxiliary
	// namespace is the namespace of Application which is used to set the namespace for Crossplane connection secret,
	// ComponentDefinition/TratiDefinition OpenAPI v3 schema
	namespace string
	// outputSecretName is used to store all secret names which are generated by cloud resource components
	outputSecretName string
	// requiredSecrets is used to store all secret names which are generated by cloud resource components and required by current component
	requiredSecrets []RequiredSecrets
}

// RequiredSecrets is used to store all secret names which are generated by cloud resource components and required by current component
type RequiredSecrets struct {
	Namespace   string
	Name        string
	ContextName string
	Data        map[string]interface{}
}

// NewContext create render templateContext
func NewContext(namespace, name, appName, appRevision string) Context {
	return &templateContext{
		name:        name,
		appName:     appName,
		appRevision: appRevision,
		configs:     []map[string]string{},
		auxiliaries: []Auxiliary{},
		namespace:   namespace,
	}
}

// SetBase set templateContext base model
func (ctx *templateContext) SetConfigs(configs []map[string]string) {
	ctx.configs = configs
}

// SetBase set templateContext base model
func (ctx *templateContext) SetBase(base model.Instance) {
	ctx.base = base
}

// AppendAuxiliaries add Assist model to templateContext
func (ctx *templateContext) AppendAuxiliaries(auxiliaries ...Auxiliary) {
	ctx.auxiliaries = append(ctx.auxiliaries, auxiliaries...)
}

// BaseContextFile return cue format string of templateContext
func (ctx *templateContext) BaseContextFile() string {
	var buff string
	buff += fmt.Sprintf(ContextName+": \"%s\"\n", ctx.name)
	buff += fmt.Sprintf(ContextAppName+": \"%s\"\n", ctx.appName)
	buff += fmt.Sprintf(ContextAppRevision+": \"%s\"\n", ctx.appRevision)
	buff += fmt.Sprintf(ContextAppRevisionNum+": %s\n", extractRevisionNum(ctx.appRevision))
	buff += fmt.Sprintf(ContextNamespace+": \"%s\"\n", ctx.namespace)

	if ctx.base != nil {
		buff += fmt.Sprintf(OutputFieldName+": %s\n", structMarshal(ctx.base.String()))
	}

	if len(ctx.auxiliaries) > 0 {
		var auxLines []string
		for _, auxiliary := range ctx.auxiliaries {
			auxLines = append(auxLines, fmt.Sprintf("%s: %s", auxiliary.Name, structMarshal(auxiliary.Ins.String())))
		}
		if len(auxLines) > 0 {
			buff += fmt.Sprintf(OutputsFieldName+": {%s}\n", strings.Join(auxLines, "\n"))
		}
	}

	if len(ctx.configs) > 0 {
		bt, _ := json.Marshal(ctx.configs)
		buff += ConfigFieldName + ": " + string(bt) + "\n"
	}

	if len(ctx.requiredSecrets) > 0 {
		for _, s := range ctx.requiredSecrets {
			data, _ := json.Marshal(s.Data)
			buff += s.ContextName + ":" + string(data) + "\n"
		}
	}

	if ctx.outputSecretName != "" {
		buff += fmt.Sprintf("%s:\"%s\"", OutputSecretName, ctx.outputSecretName)
	}
	return fmt.Sprintf("context: %s", structMarshal(buff))
}

// ExtendedContextFile return cue format string of templateContext and extended secret context
func (ctx *templateContext) ExtendedContextFile() string {
	context := ctx.BaseContextFile()

	var bareSecret string
	if len(ctx.requiredSecrets) > 0 {
		for _, s := range ctx.requiredSecrets {
			data, _ := json.Marshal(s.Data)
			bareSecret += s.ContextName + ":" + string(data) + "\n"
		}
	}
	if bareSecret != "" {
		return context + "\n" + bareSecret
	}
	return context
}

func (ctx *templateContext) BaseContextLabels() map[string]string {
	return map[string]string{
		// appName is oam.LabelAppName
		ContextAppName: ctx.appName,
		// name is oam.LabelAppComponent
		ContextName: ctx.name,
		// appRevision is oam.LabelAppRevision
		ContextAppRevision: ctx.appRevision,
	}
}

// Output return model and auxiliaries of templateContext
func (ctx *templateContext) Output() (model.Instance, []Auxiliary) {
	return ctx.base, ctx.auxiliaries
}

// InsertSecrets will add cloud resource secret stuff to context
func (ctx *templateContext) InsertSecrets(outputSecretName string, requiredSecrets []RequiredSecrets) {
	if outputSecretName != "" {
		ctx.outputSecretName = outputSecretName
	}
	if requiredSecrets != nil {
		ctx.requiredSecrets = requiredSecrets
	}
}

func structMarshal(v string) string {
	skip := false
	v = strings.TrimFunc(v, func(r rune) bool {
		if !skip {
			if unicode.IsSpace(r) {
				return true
			}
			skip = true

		}
		return false
	})

	if strings.HasPrefix(v, "{") {
		return v
	}
	return fmt.Sprintf("{%s}", v)
}

func extractRevisionNum(appRevision string) string {
	app := strings.Split(appRevision, "-")
	vision := app[len(app)-1]
	return strings.Replace(vision, "v", "", 1)
}
