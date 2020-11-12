package oam

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	plur "github.com/gertd/go-pluralize"
	"github.com/gin-gonic/gin"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	env2 "github.com/oam-dev/kubevela/pkg/utils/env"
)

func ListTraitDefinitions(workloadName *string) ([]types.Capability, error) {
	var traitList []types.Capability
	traits, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return traitList, err
	}
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return traitList, err
	}
	traitList = assembleDefinitionList(traits, workloads, workloadName)
	return traitList, nil
}

func GetTraitDefinition(workloadName *string, capabilityAlias string) (types.Capability, error) {
	var traitDef types.Capability
	traitCap, err := plugins.GetInstalledCapabilityWithCapAlias(types.TypeTrait, capabilityAlias)
	if err != nil {
		return traitDef, err
	}
	workloadsCap, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return traitDef, err
	}
	traitList := assembleDefinitionList([]types.Capability{traitCap}, workloadsCap, workloadName)
	if len(traitList) != 1 {
		return traitDef, fmt.Errorf("could not get installed capability by %s", capabilityAlias)
	}
	traitDef = traitList[0]
	return traitDef, nil
}

func assembleDefinitionList(traits []types.Capability, workloads []types.Capability, workloadName *string) []types.Capability {
	var traitList []types.Capability
	for _, t := range traits {
		convertedApplyTo := ConvertApplyTo(t.AppliesTo, workloads)
		if *workloadName != "" {
			if !In(convertedApplyTo, *workloadName) {
				continue
			}
			convertedApplyTo = []string{*workloadName}
		}
		t.AppliesTo = convertedApplyTo
		traitList = append(traitList, t)
	}
	return traitList
}

func ConvertApplyTo(applyTo []string, workloads []types.Capability) []string {
	var converted []string
	for _, v := range applyTo {
		newName, exist := check(v, workloads)
		if !exist {
			continue
		}
		if !In(converted, newName) {
			converted = append(converted, newName)
		}
	}
	return converted
}

func check(applyto string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if Parse(applyto) == v.CrdName || Parse(applyto) == v.Name {
			return v.Name, true
		}
	}
	return "", false
}

func In(l []string, v string) bool {
	for _, ll := range l {
		if ll == v {
			return true
		}
	}
	return false
}

func Parse(applyTo string) string {
	l := strings.Split(applyTo, "/")
	if len(l) != 2 {
		return applyTo
	}
	apigroup, versionKind := l[0], l[1]
	l = strings.Split(versionKind, ".")
	if len(l) != 2 {
		return applyTo
	}
	return plur.NewClient().Plural(strings.ToLower(l[1])) + "." + apigroup
}

func SimplifyCapabilityStruct(capabilityList []types.Capability) []apis.TraitMeta {
	var traitList []apis.TraitMeta
	for _, c := range capabilityList {
		traitList = append(traitList, apis.TraitMeta{
			Name:        c.Name,
			Description: c.Description,
			AppliesTo:   c.AppliesTo,
		})
	}
	return traitList
}

func ValidateAndMutateForCore(traitType, workloadName string, flags *pflag.FlagSet, env *types.EnvMeta) error {
	switch traitType {
	case "route":
		domain, _ := flags.GetString("domain")
		if domain == "" {
			if env.Domain == "" {
				return fmt.Errorf("--domain is required if not contain in environment")
			}
			if strings.HasPrefix(env.Domain, "https://") {
				env.Domain = strings.TrimPrefix(env.Domain, "https://")
			}
			if strings.HasPrefix(env.Domain, "http://") {
				env.Domain = strings.TrimPrefix(env.Domain, "http://")
			}
			if err := flags.Set("domain", workloadName+"."+env.Domain); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %v, please make sure your template is right", err)
			}
		}
		issuer, _ := flags.GetString("issuer")
		if issuer == "" && env.Issuer != "" {
			if err := flags.Set("issuer", env.Issuer); err != nil {
				return fmt.Errorf("set flag for vela-core trait('route') err %v, please make sure your template is right", err)
			}
		}
	}
	return nil
}

//AddOrUpdateTrait attach trait to workload
func AddOrUpdateTrait(env *types.EnvMeta, appName string, componentName string, flagSet *pflag.FlagSet, template types.Capability) (*application.Application, error) {
	err := ValidateAndMutateForCore(template.Name, componentName, flagSet, env)
	if err != nil {
		return nil, err
	}
	if appName == "" {
		appName = componentName
	}
	app, err := application.Load(env.Name, appName)
	if err != nil {
		return app, err
	}
	traitAlias := template.Name
	traitData, err := app.GetTraitsByType(componentName, traitAlias)
	if err != nil {
		return app, err
	}
	for _, v := range template.Parameters {
		name := v.Name
		if v.Alias != "" {
			name = v.Alias
		}
		switch v.Type {
		case cue.IntKind:
			traitData[v.Name], err = flagSet.GetInt64(name)
		case cue.StringKind:
			traitData[v.Name], err = flagSet.GetString(name)
		case cue.BoolKind:
			traitData[v.Name], err = flagSet.GetBool(name)
		case cue.NumberKind, cue.FloatKind:
			traitData[v.Name], err = flagSet.GetFloat64(name)
		}

		if err != nil {
			return nil, fmt.Errorf("get flag(s) \"%s\" err %v", name, err)
		}
	}
	if err = app.SetTrait(componentName, traitAlias, traitData); err != nil {
		return app, err
	}
	return app, app.Save(env.Name)
}

func AttachTrait(c *gin.Context, body apis.TraitBody) (string, error) {
	// Prepare
	var appObj *application.Application
	fs := pflag.NewFlagSet("trait", pflag.ContinueOnError)
	for _, f := range body.Flags {
		fs.String(f.Name, f.Value, "")
	}
	var staging = false
	var err error
	if body.Staging != "" {
		staging, err = strconv.ParseBool(body.Staging)
		if err != nil {
			return "", err
		}
	}
	traitAlias := body.Name
	template, err := plugins.GetInstalledCapabilityWithCapAlias(types.TypeTrait, traitAlias)
	if err != nil {
		return "", err
	}
	// Run step
	env, err := env2.GetEnvByName(body.EnvName)
	if err != nil {
		return "", err
	}

	appObj, err = AddOrUpdateTrait(env, body.AppName, body.ComponentName, fs, template)
	if err != nil {
		return "", err
	}
	kubeClient := c.MustGet("KubeClient")
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	return TraitOperationRun(c, kubeClient.(client.Client), env, appObj, staging, io)
}

func TraitOperationRun(ctx context.Context, c client.Client, env *types.EnvMeta, appObj *application.Application,
	staging bool, io cmdutil.IOStreams) (string, error) {
	if staging {
		return "Staging saved", nil
	}
	err := appObj.BuildRun(ctx, c, env, io)
	if err != nil {
		return "", err
	}
	return "Deployed!", nil
}

func PrepareDetachTrait(envName string, traitType string, componentName string, appName string) (*application.Application, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = componentName
	}
	if appObj, err = application.Load(envName, appName); err != nil {
		return appObj, err
	}

	if err = appObj.RemoveTrait(componentName, traitType); err != nil {
		return appObj, err
	}
	return appObj, appObj.Save(envName)
}

func DetachTrait(c *gin.Context, envName string, traitType string, componentName string, appName string, staging bool) (string, error) {
	var appObj *application.Application
	var err error
	if appName == "" {
		appName = componentName
	}
	if appObj, err = PrepareDetachTrait(envName, traitType, componentName, appName); err != nil {
		return "", err
	}
	// Run
	env, err := env2.GetEnvByName(envName)
	if err != nil {
		return "", err
	}
	kubeClient := c.MustGet("KubeClient")
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	return TraitOperationRun(c, kubeClient.(client.Client), env, appObj, staging, io)
}
