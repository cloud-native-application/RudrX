package appfile

import (
	"errors"
	"io/ioutil"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	ErrImageNotDefined = errors.New("image not defined")
)

const DefaultAppfilePath = "./vela.yaml"

type AppFile struct {
	Name       string             `json:"name"`
	CreateTime time.Time          `json:"createTime,omitempty"`
	UpdateTime time.Time          `json:"updateTime,omitempty"`
	Services   map[string]Service `json:"services"`
	Secrets    map[string]string  `json:"secrets,omitempty"`

	configGetter configGetter
}

func NewAppFile() *AppFile {
	return &AppFile{
		Services:     make(map[string]Service),
		Secrets:      make(map[string]string),
		configGetter: defaultConfigGetter{},
	}
}

func Load() (*AppFile, error) {
	return LoadFromFile(DefaultAppfilePath)
}

func LoadFromFile(filename string) (*AppFile, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	af := NewAppFile()
	err = yaml.Unmarshal(b, af)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// BuildOAM renders Appfile into AppConfig, Components. It also builds images for services if defined.
func (app *AppFile) BuildOAM(ns string, io cmdutil.IOStreams, tm template.Manager, slience bool) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	return app.buildOAM(ns, io, true, tm, slience)
}

// RenderOAM renders Appfile into AppConfig, Components.
func (app *AppFile) RenderOAM(ns string, io cmdutil.IOStreams, tm template.Manager, silence bool) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	return app.buildOAM(ns, io, false, tm, silence)
}

func (app *AppFile) buildOAM(ns string, io cmdutil.IOStreams, buildImage bool, tm template.Manager, silence bool) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {

	appConfig := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: ns,
		},
	}

	var comps []*v1alpha2.Component

	for sname, svc := range app.GetServices() {
		var image string
		v, ok := svc["image"]
		if ok {
			image = v.(string)
		}

		if b := svc.GetBuild(); b != nil {
			if image == "" {
				return nil, nil, nil, ErrImageNotDefined
			}
			if buildImage {
				io.Infof("\nBuilding service (%s)...\n", sname)
				if err := b.BuildImage(io, image); err != nil {
					return nil, nil, nil, err
				}
			}
		}
		if !silence {
			io.Infof("\nRendering configs for service (%s)...\n", sname)
		}
		acComp, comp, err := svc.RenderService(tm, sname, ns, app.configGetter)
		if err != nil {
			return nil, nil, nil, err
		}
		appConfig.Spec.Components = append(appConfig.Spec.Components, *acComp)
		comps = append(comps, comp)
	}

	addWorkloadTypeLabel(comps, app.Services)
	health := addHealthScope(appConfig)
	return comps, appConfig, []oam.Object{health}, nil
}

func addWorkloadTypeLabel(comps []*v1alpha2.Component, services map[string]Service) {
	for _, comp := range comps {
		workloadType := services[comp.Name].GetType()
		workloadObject := comp.Spec.Workload.Object.(*unstructured.Unstructured)
		labels := workloadObject.GetLabels()
		if labels == nil {
			labels = map[string]string{oam.WorkloadTypeLabel: workloadType}
		} else {
			labels[oam.WorkloadTypeLabel] = workloadType
		}
		workloadObject.SetLabels(labels)
	}
}

func addHealthScope(appConfig *v1alpha2.ApplicationConfiguration) *v1alpha2.HealthScope {
	health := &v1alpha2.HealthScope{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.HealthScopeGroupVersionKind.GroupVersion().String(),
			Kind:       v1alpha2.HealthScopeKind,
		},
	}
	health.Name = FormatDefaultHealthScopeName(appConfig.Name)
	health.Namespace = appConfig.Namespace
	health.Spec.WorkloadReferences = make([]v1alpha1.TypedReference, 0)
	for i := range appConfig.Spec.Components {
		// TODO(wonderflow): Temporarily we add health scope here, should change to use scope framework
		appConfig.Spec.Components[i].Scopes = append(appConfig.Spec.Components[i].Scopes, v1alpha2.ComponentScope{
			ScopeReference: v1alpha1.TypedReference{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.HealthScopeKind,
				Name:       health.Name,
			},
		})
	}
	return health
}
func FormatDefaultHealthScopeName(appName string) string {
	return appName + "-default-health"
}
