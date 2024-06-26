package subaccountsync

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func configureInformer(informer *cache.SharedIndexInformer, stateReconciler *stateReconcilerType, logger *slog.Logger, metrics *Metrics) {
	_, err := (*informer).AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			metrics.informer.With(prometheus.Labels{"event": "add"}).Inc()
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				logger.Error(fmt.Sprintf("added Kyma resource is not an Unstructured: %s", obj))
				return
			}
			subaccountID, runtimeID, betaEnabled := getDataFromLabels(u)
			if subaccountID == "" {
				logger.Error(fmt.Sprintf("added Kyma resource has no subaccount label: %s", u.GetName()))
				return
			}
			stateReconciler.reconcileResourceUpdate(subaccountIDType(subaccountID), runtimeIDType(runtimeID), runtimeStateType{betaEnabled: betaEnabled})
			data, err := stateReconciler.accountsClient.GetSubaccountData(subaccountID)
			if err != nil {
				logger.Warn(fmt.Sprintf("while getting data for subaccount:%s", err))
			} else {
				stateReconciler.reconcileCisAccount(subaccountIDType(subaccountID), data)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			metrics.informer.With(prometheus.Labels{"event": "update"}).Inc()
			u, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				logger.Error(fmt.Sprintf("updated Kyma resource is not an Unstructured: %s", newObj))
				return
			}
			subaccountID, runtimeID, betaEnabled := getDataFromLabels(u)
			if subaccountID == "" {
				logger.Error(fmt.Sprintf("updated Kyma resource has no subaccount label: %s", u.GetName()))
				return
			}
			if !reflect.DeepEqual(oldObj.(*unstructured.Unstructured).GetLabels(), u.GetLabels()) {
				stateReconciler.reconcileResourceUpdate(subaccountIDType(subaccountID), runtimeIDType(runtimeID), runtimeStateType{betaEnabled: betaEnabled})
			}
		},
		DeleteFunc: func(obj interface{}) {
			metrics.informer.With(prometheus.Labels{"event": "delete"}).Inc()
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				logger.Error(fmt.Sprintf("deleted Kyma resource is not an Unstructured: %s", obj))
				return
			}
			logger.Info(fmt.Sprintf("Kyma resource deleted: %s", u.GetName()))
			subaccountID, runtimeID, _ := getDataFromLabels(u)
			if subaccountID == "" || runtimeID == "" {
				// deleted kyma resource without subaccount label or runtime label - no need to make fuss, silently ignore
				return
			}
			stateReconciler.deleteRuntimeFromState(subaccountIDType(subaccountID), runtimeIDType(runtimeID))
		},
	})
	fatalOnError(err)
}
