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

package containerizedworkload

import (
	"context"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// create a corresponding deployment
func (r *Reconciler) renderDeployment(ctx context.Context,
	workload *v1alpha2.ContainerizedWorkload) (*appsv1.Deployment, error) {

	resources, err := TranslateContainerWorkload(ctx, workload)
	if err != nil {
		return nil, err
	}

	deploy, ok := resources[0].(*appsv1.Deployment)
	if !ok {
		return nil, errors.New("internal error, deployment is not rendered correctly")
	}
	// make sure we don't have opinion on the replica count
	deploy.Spec.Replicas = nil
	// k8s server-side patch complains if the protocol is not set
	for i := 0; i < len(deploy.Spec.Template.Spec.Containers); i++ {
		for j := 0; j < len(deploy.Spec.Template.Spec.Containers[i].Ports); j++ {
			if len(deploy.Spec.Template.Spec.Containers[i].Ports[j].Protocol) == 0 {
				deploy.Spec.Template.Spec.Containers[i].Ports[j].Protocol = corev1.ProtocolTCP
			}
		}
	}
	klog.InfoS("Rendered a deployment", "deploy", deploy.Spec.Template.Spec)

	// set the controller reference so that we can watch this deployment and it will be deleted automatically
	if err := ctrl.SetControllerReference(workload, deploy, r.Scheme); err != nil {
		return nil, err
	}

	return deploy, nil
}

// create a service for the deployment
func (r *Reconciler) renderService(ctx context.Context,
	workload *v1alpha2.ContainerizedWorkload, deploy *appsv1.Deployment) (*corev1.Service, error) {
	// create a service for the workload
	resources, err := ServiceInjector(ctx, workload, []oam.Object{deploy})
	if err != nil {
		return nil, err
	}
	service, ok := resources[1].(*corev1.Service)
	if !ok {
		return nil, errors.New("internal error, service is not rendered correctly")
	}
	// the service injector lib doesn't set the namespace and serviceType
	service.Namespace = workload.Namespace
	service.Spec.Type = corev1.ServiceTypeClusterIP
	// k8s server-side patch complains if the protocol is not set
	for i := 0; i < len(service.Spec.Ports); i++ {
		service.Spec.Ports[i].Protocol = corev1.ProtocolTCP
	}
	// always set the controller reference so that we can watch this service and
	if err := ctrl.SetControllerReference(workload, service, r.Scheme); err != nil {
		return nil, err
	}
	return service, nil
}

// create ConfigMaps for ContainerConfigFiles
func (r *Reconciler) renderConfigMaps(ctx context.Context,
	workload *v1alpha2.ContainerizedWorkload, deploy *appsv1.Deployment) ([]*corev1.ConfigMap, error) {
	configMaps, err := TranslateConfigMaps(ctx, workload)
	if err != nil {
		return nil, err
	}
	for _, cm := range configMaps {
		// always set the controller reference so that we can watch this configmap and it will be deleted automatically
		if err := ctrl.SetControllerReference(deploy, cm, r.Scheme); err != nil {
			return nil, err
		}
	}
	return configMaps, nil
}

// delete deployments/services that are not the same as the existing
// nolint:gocyclo
func (r *Reconciler) cleanupResources(ctx context.Context,
	workload *v1alpha2.ContainerizedWorkload, deployUID, serviceUID *types.UID) error {
	klog.InfoS("GC deployment", "workload", klog.KObj(workload))
	var deploy appsv1.Deployment
	var service corev1.Service
	for _, res := range workload.Status.Resources {
		uid := res.UID
		if res.Kind == util.KindDeployment && res.APIVersion == appsv1.SchemeGroupVersion.String() {
			if uid != *deployUID {
				klog.InfoS("Found an orphaned deployment", "deployment UID", *deployUID, "orphaned  UID", uid)
				dn := client.ObjectKey{Name: res.Name, Namespace: workload.Namespace}
				if err := r.Get(ctx, dn, &deploy); err != nil {
					if apierrors.IsNotFound(err) {
						continue
					}
					return err
				}
				if err := r.Delete(ctx, &deploy); err != nil {
					return err
				}
				klog.InfoS("Removed an orphaned deployment", "deployment UID", *deployUID, "orphaned UID", uid)
			}
		} else if res.Kind == util.KindService && res.APIVersion == corev1.SchemeGroupVersion.String() {
			if uid != *serviceUID {
				klog.InfoS("Found an orphaned service", "orphaned  UID", uid)
				sn := client.ObjectKey{Name: res.Name, Namespace: workload.Namespace}
				if err := r.Get(ctx, sn, &service); err != nil {
					if apierrors.IsNotFound(err) {
						continue
					}
					return err
				}
				if err := r.Delete(ctx, &service); err != nil {
					return err
				}
				klog.InfoS("Removed an orphaned service", "orphaned UID", uid)
			}
		}
	}
	return nil
}
