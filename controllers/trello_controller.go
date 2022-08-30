package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adlio/trello"
	"github.com/hashicorp/go-retryablehttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var FinalizerName = "trelloctrl.e13.dev/finalizer"

type TrelloReconciler struct {
	httpClient   *retryablehttp.Client
	c            client.Client
	target       metav1.TypeMeta
	trello       *trello.Client
	trelloListID string
}

func NewTrelloReconciler(apiKey, apiToken, trelloListID, targetAPIVersion, targetKind string, c client.Client) *TrelloReconciler {
	hc := retryablehttp.NewClient()

	hc.HTTPClient.Timeout = 15 * time.Second
	hc.RetryWaitMin = 2 * time.Second
	hc.RetryWaitMax = 30 * time.Second
	hc.RetryMax = 4
	hc.Logger = nil

	return &TrelloReconciler{
		httpClient: hc,
		c:          c,
		trello:     trello.NewClient(apiKey, apiToken),
		target: metav1.TypeMeta{
			// APIVersion: "kustomize.toolkit.fluxcd.io/v1beta2",
			// Kind:       "Kustomization",
			APIVersion: targetAPIVersion,
			Kind:       targetKind,
		},
		trelloListID: trelloListID,
	}
}

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch

func (r *TrelloReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName.String())

	obj, err := r.getResource(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to query resource: %w", err)
	}

	if !controllerutil.ContainsFinalizer(obj, FinalizerName) {
		patch := client.MergeFrom(obj.DeepCopy())
		controllerutil.AddFinalizer(obj, FinalizerName)
		return ctrl.Result{Requeue: true}, r.c.Patch(ctx, obj, patch, client.FieldOwner("notification-agent-controller"))
	}

	list, err := r.trello.GetList(r.trelloListID)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch Trello list: %w", err)
	}
	cards, err := list.GetCards()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to fetch Trello cards from list: %w", err)
	}

	cardName := req.NamespacedName.String() + " "
	var card *trello.Card
	for _, existingCard := range cards {
		if strings.HasPrefix(existingCard.Name, cardName) {
			card = existingCard
		}
	}

	if obj.GetDeletionTimestamp() != nil {
		// the object is being deleted, remove it's replica from the Trello list and then remove the finalizer
		if card != nil { // card may be nil if the previous attempt to remove the finalizer failed
			if err := card.Delete(); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to delete card for deleted object: %w", err)
			}
		}
		patch := client.MergeFrom(obj.DeepCopy())
		controllerutil.RemoveFinalizer(obj, FinalizerName)
		return ctrl.Result{Requeue: true}, r.c.Patch(ctx, obj, patch, client.FieldOwner("notification-agent-controller"))
	}

	if card == nil {
		logger.Info(fmt.Sprintf("card with prefix %s not found", cardName))
		card = &trello.Card{
			Name:   cardName,
			IDList: r.trelloListID,
		}
	}

	ready, err := r.isReady(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed checking ready state: %w", err)
	}

	nameWithoutIndicator := strings.Split(card.Name, " ")[0]
	if ready {
		card.Name = nameWithoutIndicator + " ✅"
	} else {
		card.Name = nameWithoutIndicator + " ❌"
	}

	if card.ID != "" {
		logger.Info("updating card", "card", card.Name)
		if err := card.Update(trello.Arguments{"name": card.Name}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update Trello card: %w", err)
		}
	} else {
		logger.Info("creating new card")
		if err := r.trello.CreateCard(card); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create Trello card: %w", err)
		}
	}

	logger.Info("done")
	return ctrl.Result{}, nil
}

func (r *TrelloReconciler) getResource(ctx context.Context, n types.NamespacedName) (*unstructured.Unstructured, error) {
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(r.target.GroupVersionKind())
	if err := r.c.Get(ctx, n, target); err != nil {
		return nil, fmt.Errorf("unable to get resource %s: %w", n.String(), err)
	}

	return target, nil
}

func (r *TrelloReconciler) isReady(ctx context.Context, n types.NamespacedName) (bool, error) {
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(r.target.GroupVersionKind())
	if err := r.c.Get(ctx, n, target); err != nil {
		return false, fmt.Errorf("unable to get resource %s: %w", n.String(), err)
	}

	res, err := status.Compute(target)
	if err != nil {
		return false, fmt.Errorf("unable to compute status: %w", err)
	}

	return res.Status == status.CurrentStatus, nil

}

func (r *TrelloReconciler) SetupWithManager(mgr ctrl.Manager) error {
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(r.target.GroupVersionKind())

	return ctrl.NewControllerManagedBy(mgr).
		For(target).
		Complete(r)
}
