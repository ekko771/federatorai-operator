package utils

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"strings"
	"time"

	prom_op_api "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"

	federatoraiv1alpha1 "github.com/containers-ai/federatorai-operator/pkg/apis/federatorai/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetValidAlamedaService(k8sClient client.Client) (*federatoraiv1alpha1.AlamedaService, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alamedaServiceList := &federatoraiv1alpha1.AlamedaServiceList{}
	if err := k8sClient.List(ctx, alamedaServiceList); err != nil {
		return nil, err
	}

	if len(alamedaServiceList.Items) == 0 {
		return nil, fmt.Errorf("No AlamedaService found")
	}

	var validAlamSvc *federatoraiv1alpha1.AlamedaService
	for _, alamSvc := range alamedaServiceList.Items {
		if validAlamSvc == nil || alamSvc.GetCreationTimestamp().Unix() < validAlamSvc.GetCreationTimestamp().Unix() {
			validAlamSvc = &alamSvc
		}
	}
	return validAlamSvc, nil
}

func GetPromSvcURLFromAlamedaService(alamSvc *federatoraiv1alpha1.AlamedaService) string {
	return alamSvc.Spec.PrometheusService
}

func GetPromSvcNamespacedNamedBySvcURL(svcURL string) (string, string) {
	sps := strings.Split(svcURL, ".")
	if len(sps) < 2 {
		return "", ""
	}

	svcName := sps[0]
	if spSvcs := strings.Split(svcName, "//"); len(spSvcs) > 0 {
		svcName = spSvcs[len(spSvcs)-1]
	}
	svcNS := sps[1]
	svcNS = strings.Split(svcNS, ":")[0]

	if len(sps) == 2 {
		return svcNS, svcName
	}
	if len(sps) > 2 {
		svcStr := strings.Split(sps[2], ":")[0]
		if svcStr != "svc" {
			svcName = svcNS
			svcNS = svcStr
		}
	}
	return svcNS, svcName
}

func IsPromSrvDeployedByOperator(k8sClient client.Client) (bool, error) {
	prometheusRuleCRD := &apiextensionsv1beta1.CustomResourceDefinition{}
	err := GetResource(k8sClient, types.NamespacedName{
		Name: "prometheusrules.monitoring.coreos.com",
	}, prometheusRuleCRD)
	if err == nil {
		return true, nil
	} else if k8sErrors.IsNotFound(err) {
		return false, nil
	} else {
		return false, err
	}
}

func GetPromSrvDeploy(k8sClient client.Client, promNS string) (*appsv1.Deployment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	promDeployList := &appsv1.DeploymentList{}
	if err := k8sClient.List(ctx, promDeployList, client.InNamespace(promNS)); err != nil {
		return nil, err
	}

	if len(promDeployList.Items) == 0 {
		return nil, fmt.Errorf("No any prometheus related deployment found")
	}

	for _, promDeploy := range promDeployList.Items {
		promSrvCt, err := getPromSrvContainer(&promDeploy)
		if promSrvCt != nil && err == nil {
			return &promDeploy, err
		}
	}
	return nil, fmt.Errorf("No matched prometheus server deployment found")
}

func getPromSrvContainer(promSrvDeploy *appsv1.Deployment) (*corev1.Container, error) {
	imgMatched := false
	argMatched := false
	for _, ct := range promSrvDeploy.Spec.Template.Spec.Containers {
		if strings.Contains(ct.Image, "prom/prometheus") {
			imgMatched = true
		}
		for _, arg := range ct.Args {
			if strings.Contains(arg, "--config.file") {
				argMatched = true
			}
		}
		if imgMatched && argMatched {
			return &ct, nil
		}
	}
	return nil, fmt.Errorf("No matched prometheus server container found in deployment")
}

func GetPromSrvConfigMap(k8sClient client.Client, promSrvDeploy *appsv1.Deployment) (*corev1.ConfigMap, error) {
	/*
		apiVersion: apps/v1
		kind: Deployment
		spec:
		  template:
			spec:
			  containers:
			  - args:
				- --config.file=/etc/config/prometheus.yml
				volumeMounts:
			    - mountPath: /etc/config
			      name: config-volume
			  volumes:
		      - configMap:
		          name: prometheus-server
		        name: config-volume
	*/

	promSrvCt, err := getPromSrvContainer(promSrvDeploy)
	if err != nil {
		return nil, err
	}

	cfgCMName := ""
OuterForLabel:
	for _, arg := range promSrvCt.Args {
		if !strings.Contains(arg, "--config.file") {
			continue
		}
		for _, vm := range promSrvCt.VolumeMounts {
			if strings.Contains(arg, vm.MountPath) {
				sps := strings.Split(arg, fmt.Sprintf("%s/", vm.MountPath))
				if len(sps) != 2 {
					continue
				}
				if strings.Contains(sps[1], "/") {
					continue
				}
				for _, volume := range promSrvDeploy.Spec.Template.Spec.Volumes {
					if volume.Name == vm.Name && volume.ConfigMap != nil {
						cfgCMName = volume.ConfigMap.Name
						break OuterForLabel
					}
				}
			}
		}
	}
	if cfgCMName == "" {
		return nil, fmt.Errorf("No prometheus server config volume found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	promSrvConfigMap := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: promSrvDeploy.GetNamespace(),
		Name:      cfgCMName,
	}, promSrvConfigMap); err != nil {
		return nil, err
	}
	return promSrvConfigMap, nil
}

func GetPromSrvCfgFileNameFromCM(promSrvDeploy *appsv1.Deployment, promSrvConfigMap *corev1.ConfigMap) string {
	promSrvCt, err := getPromSrvContainer(promSrvDeploy)
	if err != nil {
		return ""
	}
	for _, arg := range promSrvCt.Args {
		if !strings.Contains(arg, "--config.file") {
			continue
		}
		for _, vm := range promSrvCt.VolumeMounts {
			if strings.Contains(arg, vm.MountPath) {
				sps := strings.Split(arg, fmt.Sprintf("%s/", vm.MountPath))
				if len(sps) != 2 {
					continue
				}
				if strings.Contains(sps[1], "/") {
					continue
				}
				return sps[1]
			}
		}
	}
	return ""
}

func GetMissingRules(allPromRules []*prom_op_api.PrometheusRule,
	patchRulesFileStr map[interface{}]interface{}) map[string]string {
	rulesMap := map[string]string{}
	existingRecords := map[string]bool{}
	for _, promRule := range allPromRules {
		for _, promGrp := range promRule.Spec.Groups {
			for _, rule := range promGrp.Rules {
				if rule.Alert == "" && rule.Record != "" {
					existingRecords[rule.Record] = true
				}
			}
		}
	}
	for _, grp := range patchRulesFileStr["groups"].([]interface{}) {
		grpContent := grp.(map[string]interface{})
		for key, value := range grpContent {
			if key == "rules" {
				rules := value.([]interface{})
				for _, ruleContent := range rules {
					recordName, recOK := ruleContent.(map[string]interface{})["record"]
					expr, exprOK := ruleContent.(map[string]interface{})["expr"]
					if _, ok := existingRecords[recordName.(string)]; ok {
						continue
					}
					if recOK && exprOK {
						rulesMap[recordName.(string)] = expr.(string)
					}
				}
			}
		}
	}
	return rulesMap
}

func IsNamespaceSelected(k8sClient client.Client, selector labels.Selector, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nsList := &corev1.NamespaceList{}
	if err := k8sClient.List(ctx, nsList, client.MatchingLabelsSelector{
		Selector: selector,
	}); err != nil {
		return false, err
	}

	for _, ns := range nsList.Items {
		if ns.GetName() == name {
			return true, nil
		}
	}
	return false, nil
}

func GenLabelMapByLabelSelector(selector labels.Selector) (map[string]string, bool) {
	labelMap := map[string]string{}
	reqs, selectable := selector.Requirements()
	if !selectable {
		return nil, selectable
	}
	for _, req := range reqs {
		labelKey := req.Key()
		if req.Operator() == selection.In {
			for _, val := range req.Values().List() {
				labelMap[labelKey] = val
			}
		} else if req.Operator() == selection.Exists {
			labelMap[labelKey] = ""
		} else if req.Operator() == selection.Equals {
			for _, val := range req.Values().List() {
				labelMap[labelKey] = val
			}
		}
	}
	return labelMap, selectable
}

func GUnZip(data []byte) ([]byte, error) {
	if gzipReader, err := gzip.NewReader(bytes.NewBuffer(data)); err != nil {
		return nil, err
	} else {
		bBuf := bytes.Buffer{}
		if _, err = bBuf.ReadFrom(gzipReader); err != nil {
			return nil, err
		}
		return bBuf.Bytes(), nil
	}
}

func GZip(data []byte) ([]byte, error) {
	bBuf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(bBuf)

	if _, err := gzWriter.Write(data); err != nil {
		return nil, err
	}

	if err := gzWriter.Flush(); err != nil {
		return nil, err
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return bBuf.Bytes(), nil
}
