package prometheus

import (
	"github.com/containers-ai/federatorai-operator/cmd/patch/utils"
	patch_utils "github.com/containers-ai/federatorai-operator/cmd/patch/utils"
	prom_op_api "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func PatchRelabelings(k8sClient client.Client) error {
	validAlamSvc, err := patch_utils.GetValidAlamedaService(k8sClient)
	if err != nil {
		return err
	}
	promSvcURL := patch_utils.GetPromSvcURLFromAlamedaService(validAlamSvc)
	promSvcNS, _ := patch_utils.GetPromSvcNamespacedNamedBySvcURL(promSvcURL)
	serviceMonitors, err := utils.ListServiceMonitors(k8sClient, client.InNamespace(promSvcNS))
	if err != nil {
		return err
	}
	for _, sm := range serviceMonitors {
		needUpdate := false
		for epIdx := range sm.Spec.Endpoints {
			for _, missingRelabelCfg := range viper.Get("patch.prometheus.metricRelabeling").([]interface{}) {
				ep := &sm.Spec.Endpoints[epIdx]
				missingRelabelCfgIns := missingRelabelCfg.(map[string]interface{})
				if ok := missingEPRelabelCheck(ep.MetricRelabelConfigs, missingRelabelCfgIns); !ok {
					if ep.MetricRelabelConfigs == nil {
						ep.MetricRelabelConfigs = []*prom_op_api.RelabelConfig{}
					}
					sourceLabels := []string{}
					for _, labelVal := range missingRelabelCfgIns["sourceLabels"].([]interface{}) {
						sourceLabels = append(sourceLabels, labelVal.(string))
					}
					ep.MetricRelabelConfigs = append(ep.MetricRelabelConfigs, &prom_op_api.RelabelConfig{
						SourceLabels: sourceLabels,
						Separator:    missingRelabelCfgIns["separator"].(string),
						TargetLabel:  missingRelabelCfgIns["targetLabel"].(string),
						Regex:        missingRelabelCfgIns["regex"].(string),
						Replacement:  missingRelabelCfgIns["replacement"].(string),
						Action:       missingRelabelCfgIns["action"].(string),
					})
					needUpdate = true
				}
			}
		}
		if needUpdate {
			err := patch_utils.UpdateResource(k8sCli, sm)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func RelabelingCheck(k8sClient client.Client) (bool, error) {
	validAlamSvc, err := patch_utils.GetValidAlamedaService(k8sClient)
	if err != nil {
		return false, err
	}
	promSvcURL := patch_utils.GetPromSvcURLFromAlamedaService(validAlamSvc)
	promSvcNS, _ := patch_utils.GetPromSvcNamespacedNamedBySvcURL(promSvcURL)
	serviceMonitors, err := utils.ListServiceMonitors(k8sClient, client.InNamespace(promSvcNS))
	if err != nil {
		return false, err
	}
	for _, sm := range serviceMonitors {
		for _, ep := range sm.Spec.Endpoints {
			for _, missingRelabelCfg := range viper.Get("patch.prometheus.metricRelabeling").([]interface{}) {
				if ok := missingEPRelabelCheck(ep.MetricRelabelConfigs, missingRelabelCfg.(map[string]interface{})); !ok {
					return ok, nil
				}
			}
		}
	}
	return true, nil
}

func missingEPRelabelCheck(epRelabelCfgs []*prom_op_api.RelabelConfig,
	missingRelabelCfg map[string]interface{}) bool {
	for _, relabelCfg := range epRelabelCfgs {
		if tarL, ok := missingRelabelCfg["targetLabel"]; ok {
			if tarL.(string) == relabelCfg.TargetLabel {
				return true
			}
		}
	}
	return false
}
