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

package application

import (
	"context"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	errUpdateApplicationStatus    = "cannot update application status"
	errUpdateApplicationFinalizer = "cannot update application finalizer"
)

const (
	resourceTrackerFinalizer = "latestResourceTracker.finalizer.core.oam.dev"
	onlyRevisionFinalizer    = "onlyRevision.finalizer.core.oam.dev"
)

// Reconciler reconciles a Application object
type Reconciler struct {
	client.Client
	dm                   discoverymapper.DiscoveryMapper
	pd                   *packages.PackageDiscover
	Scheme               *runtime.Scheme
	Recorder             event.Recorder
	applicator           apply.Applicator
	appRevisionLimit     int
	concurrentReconciles int
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=applications/status,verbs=get;update;patch

// Reconcile process app event
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	klog.InfoS("Reconcile application", "application", klog.KRef(req.Namespace, req.Name))

	app := new(v1beta1.Application)
	if err := r.Get(ctx, client.ObjectKey{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, app); err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	handler := &appHandler{
		r:   r,
		app: app,
	}
	if app.Status.LatestRevision != nil {
		// record previous app revision name
		handler.previousRevisionName = app.Status.LatestRevision.Name
	}

	if endReconcile, err := r.handleFinalizers(ctx, app); endReconcile {
		return ctrl.Result{}, err
	}

	klog.Info("Start Rendering")

	app.Status.Phase = common.ApplicationRendering

	klog.Info("Parse template")
	// parse template
	appParser := appfile.NewApplicationParser(r.Client, r.dm, r.pd)

	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)
	generatedAppfile, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		klog.InfoS("Failed to parse application", "err", err)
		app.Status.SetConditions(errorCondition("Parsed", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return handler.handleErr(err)
	}

	app.Status.SetConditions(readyCondition("Parsed"))
	handler.appfile = generatedAppfile

	appRev, err := handler.GenerateAppRevision(ctx)
	if err != nil {
		klog.InfoS("Failed to calculate appRevision", "err", err)
		app.Status.SetConditions(errorCondition("Parsed", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedParse, err))
		return handler.handleErr(err)
	}
	r.Recorder.Event(app, event.Normal(velatypes.ReasonParsed, velatypes.MessageParsed))
	// Record the revision so it can be used to render data in context.appRevision
	generatedAppfile.RevisionName = appRev.Name

	klog.Info("Build template")
	// build template to applicationconfig & component
	ac, comps, err := generatedAppfile.GenerateApplicationConfiguration()
	if err != nil {
		klog.InfoS("Failed to generate applicationConfiguration", "err", err)
		app.Status.SetConditions(errorCondition("Built", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRender, err))
		return handler.handleErr(err)
	}

	// pass the App label and annotation to ac except some app specific ones
	oamutil.PassLabelAndAnnotation(app, ac)

	app.Status.SetConditions(readyCondition("Built"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonRendered, velatypes.MessageRendered))
	klog.Info("Apply application revision & component to the cluster")
	// apply application revision & component to the cluster
	if err := handler.apply(ctx, appRev, ac, comps); err != nil {
		klog.InfoS("Failed to apply application revision & component to the cluster", "err", err)
		app.Status.SetConditions(errorCondition("Applied", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedApply, err))
		return handler.handleErr(err)
	}

	// if inplace is false and rolloutPlan is nil, it means the user will use an outer AppRollout object to rollout the application
	if handler.app.Spec.RolloutPlan != nil {
		res, err := handler.handleRollout(ctx)
		if err != nil {
			klog.InfoS("Failed to handle rollout", "err", err)
			app.Status.SetConditions(errorCondition("Rollout", err))
			r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedRollout, err))
			return handler.handleErr(err)
		}
		// skip health check and garbage collection if rollout have not finished
		// start next reconcile immediately
		if res.Requeue || res.RequeueAfter > 0 {
			app.Status.Phase = common.ApplicationRollingOut
			return res, r.UpdateStatus(ctx, app)
		}

		// there is no need reconcile immediately, that means the rollout operation have finished
		r.Recorder.Event(app, event.Normal(velatypes.ReasonRollout, velatypes.MessageRollout))
		app.Status.SetConditions(readyCondition("Rollout"))
		klog.Info("Finished rollout ")
	}

	// The following logic will be skipped if rollout have not finished
	app.Status.SetConditions(readyCondition("Applied"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonFailedApply, velatypes.MessageApplied))
	app.Status.Phase = common.ApplicationHealthChecking
	klog.Info("Check application health status")
	// check application health status
	appCompStatus, healthy, err := handler.statusAggregate(generatedAppfile)
	if err != nil {
		klog.InfoS("Failed to aggregate status", "err", err)
		app.Status.SetConditions(errorCondition("HealthCheck", err))
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedHealthCheck, err))
		return handler.handleErr(err)
	}
	if !healthy {
		app.Status.SetConditions(errorCondition("HealthCheck", errors.New("not healthy")))

		app.Status.Services = appCompStatus
		// unhealthy will check again after 10s
		return ctrl.Result{RequeueAfter: time.Second * 10}, r.Status().Update(ctx, app)
	}
	app.Status.Services = appCompStatus
	app.Status.SetConditions(readyCondition("HealthCheck"))
	r.Recorder.Event(app, event.Normal(velatypes.ReasonHealthCheck, velatypes.MessageHealthCheck))
	app.Status.Phase = common.ApplicationRunning

	err = garbageCollection(ctx, handler)
	if err != nil {
		klog.InfoS("Failed to run Garbage collection", "err", err)
		r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
	}

	// Gather status of components
	var refComps []v1alpha1.TypedReference
	for _, comp := range comps {
		refComps = append(refComps, v1alpha1.TypedReference{
			APIVersion: comp.APIVersion,
			Kind:       comp.Kind,
			Name:       comp.Name,
			UID:        app.UID,
		})
	}
	app.Status.Components = refComps
	r.Recorder.Event(app, event.Normal(velatypes.ReasonDeployed, velatypes.MessageDeployed))
	return ctrl.Result{}, r.UpdateStatus(ctx, app)
}

func (r *Reconciler) handleFinalizers(ctx context.Context, app *v1beta1.Application) (bool, error) {
	if app.ObjectMeta.DeletionTimestamp.IsZero() {
		// NOTE Because resource tracker is cluster-scoped resources, we cannot garbage collect them
		// by setting application(namespace-scoped) as their owner. So we must delete all
		// resource trackers through app controller's finalizer logic.

		// 'resourceTrackerFinalizer' is used to delete the resource tracker of the last app revision
		if !meta.FinalizerExists(&app.ObjectMeta, resourceTrackerFinalizer) {
			meta.AddFinalizer(&app.ObjectMeta, resourceTrackerFinalizer)
			klog.InfoS("Register new finalizer for application", "application", klog.KObj(app), "finalizer", resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		// 'onlyRevisionFinalizer' is used to delete all resource trackers of app revisions which
		// may be used out of the domain of app controller, e.g., AppRollout controller.
		if app.Annotations[oam.AnnotationAppRevisionOnly] == "true" ||
			len(app.Annotations[oam.AnnotationAppRollout]) != 0 || app.Spec.RolloutPlan != nil {
			if !meta.FinalizerExists(&app.ObjectMeta, onlyRevisionFinalizer) {
				meta.AddFinalizer(&app.ObjectMeta, onlyRevisionFinalizer)
				klog.InfoS("Register new finalizer for application", "application", klog.KObj(app), "finalizer", onlyRevisionFinalizer)
				return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
			}
		}
	} else {
		if meta.FinalizerExists(&app.ObjectMeta, resourceTrackerFinalizer) {
			if app.Status.LatestRevision != nil && len(app.Status.LatestRevision.Name) != 0 {
				latestTracker := &v1beta1.ResourceTracker{}
				latestTracker.SetName(dispatch.ConstructResourceTrackerName(app.Status.LatestRevision.Name, app.Namespace))
				if err := r.Client.Delete(ctx, latestTracker); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete latest resource tracker", "name", latestTracker.Name)
					app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
					return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
				}
			}
			meta.RemoveFinalizer(app, resourceTrackerFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
		if meta.FinalizerExists(&app.ObjectMeta, onlyRevisionFinalizer) {
			listOpts := []client.ListOption{
				client.MatchingLabels{
					oam.LabelAppName:      app.Name,
					oam.LabelAppNamespace: app.Namespace,
				}}
			rtList := &v1beta1.ResourceTrackerList{}
			if err := r.Client.List(ctx, rtList, listOpts...); err != nil {
				klog.ErrorS(err, "Failed to list resource tracker of app", "name", app.Name)
				app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
				return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
			}
			for _, rt := range rtList.Items {
				if err := r.Client.Delete(ctx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete resource tracker", "name", rt.Name)
					app.Status.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, "error to  remove finalizer")))
					return true, errors.Wrap(r.UpdateStatus(ctx, app), errUpdateApplicationStatus)
				}
			}
			meta.RemoveFinalizer(app, onlyRevisionFinalizer)
			return true, errors.Wrap(r.Client.Update(ctx, app), errUpdateApplicationFinalizer)
		}
	}
	return false, nil
}

// SetupWithManager install to manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// If Application Own these two child objects, AC status change will notify application controller and recursively update AC again, and trigger application event again...
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&v1beta1.Application{}).
		Complete(r)
}

// UpdateStatus updates v1beta1.Application's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, app *v1beta1.Application, opts ...client.UpdateOption) error {
	status := app.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: app.Name}, app); err != nil {
			return
		}
		app.Status = status
		return r.Status().Update(ctx, app, opts...)
	})
}

// Setup adds a controller that reconciles AppRollout.
func Setup(mgr ctrl.Manager, args core.Args) error {
	reconciler := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             event.NewAPIRecorder(mgr.GetEventRecorderFor("Application")),
		dm:                   args.DiscoveryMapper,
		pd:                   args.PackageDiscover,
		applicator:           apply.NewAPIApplicator(mgr.GetClient()),
		appRevisionLimit:     args.AppRevisionLimit,
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return reconciler.SetupWithManager(mgr)
}
