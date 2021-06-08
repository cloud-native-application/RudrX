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

package dispatch

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// NewAppManifestsDispatcher creates an AppManifestsDispatcher.
func NewAppManifestsDispatcher(c client.Client, appRev *v1beta1.ApplicationRevision) *AppManifestsDispatcher {
	return &AppManifestsDispatcher{
		c:          c,
		applicator: apply.NewAPIApplicator(c),
		appRev:     appRev,
		gcHandler:  NewGCHandler(c, appRev.Namespace),
	}
}

// AppManifestsDispatcher dispatch application manifests into K8s and record the dispatched manifests' references in a
// resource tracker which is named by a particular rule: name = appRevision's Name + appRevision's namespace.
// A bundle of manifests to be dispatched MUST come from the given application revision.
type AppManifestsDispatcher struct {
	c          client.Client
	applicator apply.Applicator
	gcHandler  GarbageCollector

	appRev     *v1beta1.ApplicationRevision
	previousRT *v1beta1.ResourceTracker
	skipGC     bool

	appRevName    string
	namespace     string
	currentRTName string
	currentRT     *v1beta1.ResourceTracker
}

// EnableGC return an AppManifestsDispatcher that always do GC after dispatching resources.
// GC will calculate diff between the dispatched resouces and ones recorded in the given resource tracker.
func (a *AppManifestsDispatcher) EnableGC(rt *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if rt != nil {
		a.previousRT = rt.DeepCopy()
	}
	return a
}

// EnableUpgradeAndSkipGC return an AppManifestsDispatcher that skips GC after dispatching resources.
// For the unchanged resources, dispatcher will update their owner to the newly created resource tracker.
// It's helpful in a rollout scenario where new revision is going to create a new workload while the old one should not
// be deleted before rollout is terminated.
func (a *AppManifestsDispatcher) EnableUpgradeAndSkipGC(rt *v1beta1.ResourceTracker) *AppManifestsDispatcher {
	if rt != nil {
		a.previousRT = rt.DeepCopy()
		a.skipGC = true
	}
	return a
}

// Dispatch apply manifests into k8s and return a resource tracker recording applied manifests' references.
// If GC is enabled, it will do GC after applying.
// If 'UpgradeAndSkipGC' is enabled, it will:
// - create new resources if not exist before
// - update unchanged resources' owner from the previous resource tracker to the new one
// - skip deleting(GC) any resources
func (a *AppManifestsDispatcher) Dispatch(ctx context.Context, manifests []*unstructured.Unstructured) (*v1beta1.ResourceTracker, error) {
	if err := a.validateAndComplete(ctx); err != nil {
		return nil, err
	}
	if err := a.createOrGetResourceTracker(ctx); err != nil {
		return nil, err
	}
	if err := a.applyAndRecordManifests(ctx, manifests); err != nil {
		return nil, err
	}
	if !a.skipGC && a.previousRT != nil && a.previousRT.Name != a.currentRTName {
		if err := a.gcHandler.GarbageCollect(ctx, a.previousRT, a.currentRT); err != nil {
			return nil, errors.WithMessagef(err, "cannot do GC based on resource trackers %q and %q", a.previousRT.Name, a.currentRTName)
		}
	}
	return a.currentRT.DeepCopy(), nil
}

// ReferenceScopes add workload reference to scopes' workloadRefPath
func (a *AppManifestsDispatcher) ReferenceScopes(ctx context.Context, wlRef *v1beta1.TypedReference, scopes []*v1beta1.TypedReference) error {
	// TODO handle scopes
	return nil
}

// DereferenceScopes remove workload reference from scopes' workloadRefPath
func (a *AppManifestsDispatcher) DereferenceScopes(ctx context.Context, wlRef *v1beta1.TypedReference, scopes []*v1beta1.TypedReference) error {
	// TODO handle scopes
	return nil
}

func (a *AppManifestsDispatcher) validateAndComplete(ctx context.Context) error {
	if a.appRev == nil {
		return errors.New("given application revision is nil")
	}
	if a.appRev.Name == "" || a.appRev.Namespace == "" {
		return errors.New("given application revision has no name or namespace")
	}
	a.appRevName = a.appRev.Name
	a.namespace = a.appRev.Namespace
	a.currentRTName = ConstructResourceTrackerName(a.appRevName, a.namespace)

	// no matter GC or UpgradeAndSkipGC, it requires a valid and existing resource tracker
	if a.previousRT != nil && a.previousRT.Name != a.currentRTName {
		klog.InfoS("Validate previous resource tracker exists", "previous", klog.KObj(a.previousRT))
		gotPreviousRT := &v1beta1.ResourceTracker{}
		if err := a.c.Get(ctx, client.ObjectKey{Name: a.previousRT.Name}, gotPreviousRT); err != nil {
			return errors.Errorf("given resource tracker %q doesn't exist", a.previousRT.Name)
		}
		a.previousRT = gotPreviousRT
	}
	klog.InfoS("Given previous resource tracker is nil or same as current one, so skip GC", "appRevision", klog.KObj(a.appRev))
	return nil
}

func (a *AppManifestsDispatcher) createOrGetResourceTracker(ctx context.Context) error {
	rt := &v1beta1.ResourceTracker{}
	err := a.c.Get(ctx, client.ObjectKey{Name: a.currentRTName}, rt)
	if err == nil {
		klog.InfoS("Found a resource tracker matching current app revision", "resourceTracker", rt.Name)
		// already exists, no need to update
		// because we assume the manifests' references from a specific application revision never change
		a.currentRT = rt
		return nil
	}
	if !kerrors.IsNotFound(err) {
		return errors.Wrap(err, "cannot get resource tracker")
	}
	klog.InfoS("Going to create a resource tracker", "resourceTracker", a.currentRTName)
	rt.SetName(a.currentRTName)
	// these labels can help to list resource trackers of a specific application
	rt.SetLabels(map[string]string{
		oam.LabelAppName:      ExtractAppName(a.currentRTName, a.namespace),
		oam.LabelAppNamespace: a.namespace,
	})
	if err := a.c.Create(ctx, rt); err != nil {
		klog.ErrorS(err, "Failed to create a resource tracker", "resourceTracker", a.currentRTName)
		return errors.Wrap(err, "cannot create resource tracker")
	}
	a.currentRT = rt
	return nil
}

func (a *AppManifestsDispatcher) applyAndRecordManifests(ctx context.Context, manifests []*unstructured.Unstructured) error {
	applyOpts := []apply.ApplyOption{}
	if a.previousRT != nil && a.previousRT.Name != a.currentRTName {
		klog.InfoS("Going to apply and upgrade resources", "from", a.previousRT.Name, "to", a.currentRTName)
		// if two RT's names are different, it means dispatching operation happens in an upgrade or rollout scenario
		// in such two scenarios, for those unchanged manifests, we will
		// - check existing resources are controlled by the old resource tracker
		// - set new resource tracker as their controller owner
		applyOpts = append(applyOpts, apply.MustBeControllableBy(a.previousRT.UID))
	} else {
		applyOpts = append(applyOpts, apply.MustBeControllableBy(a.currentRT.UID))
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         v1beta1.SchemeGroupVersion.String(),
		Kind:               reflect.TypeOf(v1beta1.ResourceTracker{}).Name(),
		Name:               a.currentRT.Name,
		UID:                a.currentRT.UID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	for _, rsc := range manifests {
		// each resource applied by dispatcher MUST be controlled by resource tracker
		setOrOverrideControllerOwner(rsc, ownerRef)
		if err := a.applicator.Apply(ctx, rsc, applyOpts...); err != nil {
			klog.ErrorS(err, "Failed to apply a resource", "object",
				klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
			return errors.Wrapf(err, "cannot apply manifest, name: %q apiVersion: %q kind: %q",
				rsc.GetName(), rsc.GetAPIVersion(), rsc.GetKind())
		}
		klog.InfoS("Successfully apply a resource", "object",
			klog.KObj(rsc), "apiVersion", rsc.GetAPIVersion(), "kind", rsc.GetKind())
	}
	return a.updateResourceTrackerStatus(ctx, manifests)
}

func (a *AppManifestsDispatcher) updateResourceTrackerStatus(ctx context.Context, appliedManifests []*unstructured.Unstructured) error {
	// merge applied resources and already tracked ones
	if a.currentRT.Status.TrackedResources == nil {
		a.currentRT.Status.TrackedResources = make([]v1beta1.TypedReference, 0)
	}
	for _, rsc := range appliedManifests {
		appliedRef := v1beta1.TypedReference{
			APIVersion: rsc.GetAPIVersion(),
			Kind:       rsc.GetKind(),
			Name:       rsc.GetName(),
			Namespace:  rsc.GetNamespace(),
		}
		alreadyTracked := false
		for _, tracked := range a.currentRT.Status.TrackedResources {
			if tracked.APIVersion == appliedRef.APIVersion && tracked.Kind == appliedRef.Kind &&
				tracked.Name == appliedRef.Name && tracked.Namespace == appliedRef.Namespace {
				alreadyTracked = true
				break
			}
		}
		if !alreadyTracked {
			a.currentRT.Status.TrackedResources = append(a.currentRT.Status.TrackedResources, appliedRef)
		}
	}

	// TODO move TrackedResources from status to spec
	// update status with retry
	copyRT := a.currentRT.DeepCopy()
	sts := copyRT.Status
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = a.c.Get(ctx, client.ObjectKey{Name: a.currentRTName}, copyRT); err != nil {
			return
		}
		copyRT.Status = sts
		return a.c.Status().Update(ctx, copyRT)
	}); err != nil {
		klog.ErrorS(err, "Failed to update resource tracker status", "resourceTracker", a.currentRTName)
		return errors.Wrap(err, "cannot update resource tracker status")
	}
	klog.InfoS("Successfully update resource tracker status", "resourceTracker", a.currentRTName)
	return nil
}

func setOrOverrideControllerOwner(obj *unstructured.Unstructured, controllerOwner metav1.OwnerReference) {
	ownerRefs := []metav1.OwnerReference{controllerOwner}
	for _, owner := range obj.GetOwnerReferences() {
		if owner.Controller != nil && *owner.Controller &&
			owner.UID != controllerOwner.UID {
			owner.Controller = pointer.BoolPtr(false)
		}
		ownerRefs = append(ownerRefs, owner)
	}
	obj.SetOwnerReferences(ownerRefs)
}
