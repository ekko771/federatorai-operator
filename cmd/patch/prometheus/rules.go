package prometheus

import (
	"fmt"

	"github.com/containers-ai/federatorai-operator/cmd/patch/pkg/assets"
	"github.com/containers-ai/federatorai-operator/cmd/patch/prometheus/consts"
	patch_utils "github.com/containers-ai/federatorai-operator/cmd/patch/utils"
	"github.com/containers-ai/federatorai-operator/pkg/util"
	prom_op_api "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RulesCheck(k8sClient client.Client) (bool, map[string]string, error) {
	emptyMap := map[string]string{}
	opDeploy, err := patch_utils.IsPromSrvDeployedByOperator(k8sClient)
	if err != nil {
		return false, emptyMap, err
	}

	if !opDeploy {
		return false, emptyMap, fmt.Errorf("rule patch only suport for prometheus operator")
	}

	promRules, err := patch_utils.ListPrometheusRules(k8sClient)
	if err != nil {
		return false, emptyMap, err
	}

	ruleFileBin, _ := assets.Asset(consts.PromPatchRuleFile)

	m := make(map[interface{}]interface{})

	err = yaml.Unmarshal([]byte(string(ruleFileBin)), &m)
	if err != nil {
		return false, emptyMap, err
	}

	rulesMap := patch_utils.GetMissingRules(promRules, m)
	if len(rulesMap) == 0 {
		return true, rulesMap, nil
	} else {
		return false, rulesMap, nil
	}
}

func PatchMissingRules(k8sClient client.Client, missingRulesMap map[string]string) error {
	validAlamSvc, err := patch_utils.GetValidAlamedaService(k8sClient)
	if err != nil {
		return err
	}
	promSvcURL := patch_utils.GetPromSvcURLFromAlamedaService(validAlamSvc)
	promSvcNS, _ := patch_utils.GetPromSvcNamespacedNamedBySvcURL(promSvcURL)

	prometheusList, err := patch_utils.ListPrometheuses(k8sCli, client.InNamespace(promSvcNS))
	if err != nil {
		return err
	}
	if len(prometheusList) == 0 {
		return fmt.Errorf("No prometheus CR found")
	}

	//runningNS := os.Getenv("NAMESPACE_NAME")
	runningNS := promSvcNS
	if runningNS == "" {
		runningNS = "federatorai"
	}
	/*
		secretName := viper.GetString("patch.prometheus.config.secretName")
		pmCfg, err := patch_utils.GetPromConfigFromSecret(k8sCli, runningNS, secretName)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("prometheus config: %s", pmCfg))
	*/

	runningNSIns := &corev1.Namespace{}
	if err = patch_utils.GetResource(k8sCli, client.ObjectKey{
		Name: runningNS,
	}, runningNSIns); err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(prometheusList[0].Spec.RuleNamespaceSelector)
	if err != nil {
		return err
	}
	ruleSelector, err := metav1.LabelSelectorAsSelector(prometheusList[0].Spec.RuleSelector)
	if err != nil {
		return err
	}
	pmRuleLabelMap, selectable := patch_utils.GenLabelMapByLabelSelector(ruleSelector)
	if !selectable {
		return fmt.Errorf("Prometheus rule cannot be selected by prometheus CR")
	}

	nsValid, err := patch_utils.IsNamespaceSelected(k8sCli, selector, runningNS)
	if err != nil {
		return err
	}
	if !nsValid {
		return fmt.Errorf("Prometheus rule installed in namespace %s is not selected by prometheus", runningNS)
	}
	alamPromRule := &prom_op_api.PrometheusRule{}
	err = patch_utils.GetResource(k8sCli, client.ObjectKey{
		Namespace: runningNS,
		Name:      consts.AlamedaPrometheusRuleName,
	}, alamPromRule)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	alamRuleGroup := prom_op_api.RuleGroup{
		Name:  consts.AlamedaPrometheusRuleGroupName,
		Rules: []prom_op_api.Rule{},
	}

	for rec, expr := range missingRulesMap {
		alamRuleGroup.Rules = append(alamRuleGroup.Rules, prom_op_api.Rule{
			Record: rec,
			Expr:   intstr.FromString(expr),
		})
	}

	if err != nil && k8serrors.IsNotFound(err) {
		clusterRoleGC, err := util.GetOrCreateGCClusterRole(k8sCli)
		if err != nil {
			return err
		}
		alamPromRule := &prom_op_api.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: runningNS,
				Name:      consts.AlamedaPrometheusRuleName,
				Labels:    pmRuleLabelMap,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(clusterRoleGC, rbacv1.SchemeGroupVersion.WithKind("ClusterRole")),
				},
			},
			Spec: prom_op_api.PrometheusRuleSpec{
				Groups: []prom_op_api.RuleGroup{
					alamRuleGroup,
				},
			},
		}

		err = patch_utils.CreateResource(k8sCli, alamPromRule)
		if err != nil {
			return err
		}
	} else {
		alamPromRule.ObjectMeta.Labels = pmRuleLabelMap
		if len(alamPromRule.Spec.Groups) == 0 {
			alamPromRule.Spec.Groups = []prom_op_api.RuleGroup{
				alamRuleGroup,
			}
		} else {
			for rec, expr := range missingRulesMap {
				alamPromRule.Spec.Groups[0].Rules = append(alamPromRule.Spec.Groups[0].Rules, prom_op_api.Rule{
					Record: rec,
					Expr:   intstr.FromString(expr),
				})
			}
		}
		err := patch_utils.UpdateResource(k8sCli, alamPromRule)
		if err != nil {
			return err
		}
	}

	return nil
}
