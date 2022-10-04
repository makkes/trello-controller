package controllers

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/go-logr/logr"
	"go.e13.dev/trello-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TrelloConfigReconciler struct {
	client.Client
	mgr   ctrl.Manager
	ctrls map[types.NamespacedName]context.CancelFunc
	mu    *sync.RWMutex
}

func NewTrelloConfigReconciler(c client.Client) TrelloConfigReconciler {
	return TrelloConfigReconciler{
		Client: c,
		ctrls:  make(map[types.NamespacedName]context.CancelFunc),
		mu:     &sync.RWMutex{},
	}
}

//+kubebuilder:rbac:groups=trello.e13.dev,resources=trelloconfigs,verbs=get;list;watch;patch

func (r TrelloConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName.String())
	logger.Info("reconciling")

	cfg := &v1alpha1.TrelloConfig{}
	if err := r.Get(ctx, req.NamespacedName, cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(cfg, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to create patch helper: %w", err)
	}

	// Examine if the object is under deletion
	if !cfg.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cfg, patchHelper, logger)
	}

	// Add finalizer first if not exist to avoid the race condition
	// between init and delete
	if !controllerutil.ContainsFinalizer(cfg, v1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cfg, v1alpha1.FinalizerName)
		if err := patchHelper.Patch(ctx, cfg,
			patch.WithFieldOwner("trello-controller"),
		); err != nil {
			return reconcile.Result{}, fmt.Errorf("unable to patch object: %w", err)
		}
		logger.Info("added finalizer. Re-queueing item")
		return ctrl.Result{Requeue: true}, nil
	}

	credentialsSecret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: cfg.Namespace, Name: cfg.Spec.SecretRef.Name}, credentialsSecret); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get Trello credentials Secret: %w", err)
	}
	apiKey := credentialsSecret.Data[v1alpha1.CredentialsApiKey]
	if len(apiKey) == 0 {
		return ctrl.Result{}, fmt.Errorf("empty API Key in Trello credentials Secret")
	}
	apiToken := credentialsSecret.Data[v1alpha1.CredentialsApiToken]
	if len(apiToken) == 0 {
		return ctrl.Result{}, fmt.Errorf("empty API Token in Trello credentials Secret")
	}

	controller, err := NewTrelloReconciler(
		strings.TrimSpace(string(apiKey)),
		strings.TrimSpace(string(apiToken)),
		strings.TrimSpace(cfg.Spec.ListID),
		cfg.Spec.Target.APIVersion,
		cfg.Spec.Target.Kind,
		r.mgr.GetClient(),
	).SetupWithManager(r.mgr)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to build controller: %w", err)
	}

	r.mu.RLock()
	existingCancel := r.ctrls[req.NamespacedName]
	r.mu.RUnlock()

	if existingCancel != nil {
		// A controller has already been started for this object so we
		// stop it before starting a new one.
		logger.Info("stopping controller")
		existingCancel()
		r.mu.Lock()
		delete(r.ctrls, req.NamespacedName)
		r.mu.Unlock()
		logger.Info("deleted controller from map")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.ctrls[req.NamespacedName] = cancel
	r.mu.Unlock()
	logger.Info("added controller to map")

	buf := make([]byte, 104857600)
	w := runtime.Stack(buf, true)
	if err := os.WriteFile(fmt.Sprintf("/tmp/trello-controller/stack-%d", time.Now().UnixMilli()), buf[:w], 0600); err != nil {
		logger.Error(err, "failed to write stack to file")
	}

	go func() {
		<-r.mgr.Elected()
		if err := controller.Start(ctx); err != nil {
			logger.Error(err, "unable to start controller")
			return
		}
		logger.Info("controller stopped")
		// TODO: what if the controller crashed? Do we restart it? If yes, how?
	}()

	return reconcile.Result{}, nil
}

func (r *TrelloConfigReconciler) reconcileDelete(ctx context.Context, obj *v1alpha1.TrelloConfig, patchHelper *patch.Helper, logger logr.Logger) (reconcile.Result, error) {
	nn := types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}
	if c := r.ctrls[nn]; c != nil {
		logger.Info("stopping controller")
		c()
		delete(r.ctrls, nn)
	} else {
		logger.Info("no running controller found", "ctrls", fmt.Sprintf("%#v", r.ctrls))
	}

	controllerutil.RemoveFinalizer(obj, v1alpha1.FinalizerName)
	if err := patchHelper.Patch(ctx, obj,
		patch.WithFieldOwner("trello-controller"),
	); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to patch object: %w", err)
	}

	// Stop reconciliation as the object is being deleted
	return reconcile.Result{}, nil
}

func (r *TrelloConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.mgr = mgr
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.TrelloConfig{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 4,
		}).
		Complete(r)
}
