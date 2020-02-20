package utils

import (
	"context"
	"fmt"
	"time"

	prom_op_api "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetResource(k8sClient client.Client, key client.ObjectKey, obj runtime.Object) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return k8sClient.Get(ctx, key, obj)
}

func UpdateResource(k8sClient client.Client, obj runtime.Object) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return k8sClient.Update(ctx, obj)
}

func CreateResource(k8sClient client.Client, obj runtime.Object) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return k8sClient.Create(ctx, obj)
}

func ListPrometheusRules(k8sClient client.Client, opts ...client.ListOption) ([]*prom_op_api.PrometheusRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//cannot get cache for *v1.PrometheusRuleList, its element **v1.PrometheusRule is not a runtime.Object
	promRuleList := &prom_op_api.PrometheusRuleList{}
	if err := k8sClient.List(ctx, promRuleList, opts...); err != nil {
		return nil, err
	}

	if len(promRuleList.Items) == 0 {
		return nil, fmt.Errorf("No prometheusrule CR found")
	}

	return promRuleList.Items, nil
}

func ListPrometheuses(k8sClient client.Client, opts ...client.ListOption) ([]*prom_op_api.Prometheus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	promList := &prom_op_api.PrometheusList{}
	if err := k8sClient.List(ctx, promList, opts...); err != nil {
		return nil, err
	}

	if len(promList.Items) == 0 {
		return nil, fmt.Errorf("No prometheus CR found")
	}

	return promList.Items, nil
}

func ListServiceMonitors(k8sClient client.Client, opts ...client.ListOption) ([]*prom_op_api.ServiceMonitor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	smList := &prom_op_api.ServiceMonitorList{}
	if err := k8sClient.List(ctx, smList, opts...); err != nil {
		return nil, err
	}

	if len(smList.Items) == 0 {
		return nil, fmt.Errorf("No servicemonitor CR found")
	}

	return smList.Items, nil
}
