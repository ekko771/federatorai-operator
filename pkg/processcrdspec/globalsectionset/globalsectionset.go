package globalsectionset

import (
	"strings"

	"github.com/containers-ai/federatorai-operator/pkg/apis/federatorai/v1alpha1"
	"github.com/containers-ai/federatorai-operator/pkg/processcrdspec/alamedaserviceparamter"
	"github.com/containers-ai/federatorai-operator/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func GlobalSectionSetParamterToDeployment(dep *appsv1.Deployment, asp *alamedaserviceparamter.AlamedaServiceParamter) {
	processDeploymentPrometheusService(dep, asp.PrometheusService) //Global section set DeploymentSpec's PrometheusService
	switch dep.Name {
	case util.AlamedaaiDPN:
		{
			//Global section set DeploymentSpec's image version(only alameda component)
			util.SetImageStruct(dep, asp.Version, util.AlamedaaiCTN)
			//Global section set persistentVolumeClaim to mountPath
			util.SetStorageToVolumeSource(dep, asp.Storages, "alameda-ai-type.pvc", util.AlamedaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.AlamedaaiCTN, "alameda-ai-type-storage", util.AlamedaGroup)
		}
	case util.AlamedaoperatorDPN:
		{
			util.SetImageStruct(dep, asp.Version, util.AlamedaoperatorCTN)
			util.SetStorageToVolumeSource(dep, asp.Storages, "alameda-operator-type.pvc", util.AlamedaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.AlamedaoperatorCTN, "alameda-operator-type-storage", util.AlamedaGroup)
		}
	case util.AlamedadatahubDPN:
		{
			util.SetImageStruct(dep, asp.Version, util.AlamedadatahubCTN)
			util.SetStorageToVolumeSource(dep, asp.Storages, "alameda-datahub-type.pvc", util.AlamedaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.AlamedadatahubCTN, "alameda-datahub-type-storage", util.AlamedaGroup)
		}
	case util.AlamedaevictionerDPN:
		{
			util.SetImageStruct(dep, asp.Version, util.AlamedaevictionerCTN)
			util.SetStorageToVolumeSource(dep, asp.Storages, "alameda-evictioner-type.pvc", util.AlamedaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.AlamedaevictionerCTN, "alameda-evictioner-type-storage", util.AlamedaGroup)
		}
	case util.AdmissioncontrollerDPN:
		{
			util.SetImageStruct(dep, asp.Version, util.AdmissioncontrollerCTN)
			util.SetStorageToVolumeSource(dep, asp.Storages, "admission-controller-type.pvc", util.AlamedaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.AdmissioncontrollerCTN, "admission-controller-type-storage", util.AlamedaGroup)
		}
	case util.InfluxdbDPN:
		{
			util.SetStorageToVolumeSource(dep, asp.Storages, "my-alameda.influxdb-type.pvc", util.InfluxDBGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.InfluxdbCTN, "influxdb-type-storage", util.InfluxDBGroup)
		}
	case util.GrafanaDPN:
		{
			util.SetStorageToVolumeSource(dep, asp.Storages, "my-alameda.grafana-type.pvc", util.GrafanaGroup)
			util.SetStorageToMountPath(dep, asp.Storages, util.GrafanaCTN, "grafana-type-storage", util.GrafanaGroup)
		}
	}
}

func processConfigMapsPrometheusService(cm *corev1.ConfigMap, prometheusservice string) {
	if strings.Contains(cm.Data[util.OriginComfigMapPrometheusLocation], util.OriginPrometheus_URL) && prometheusservice != "" {
		cm.Data[util.OriginComfigMapPrometheusLocation] = strings.Replace(cm.Data[util.OriginComfigMapPrometheusLocation], util.OriginPrometheus_URL, prometheusservice, -1)
	}
}

func GlobalSectionSetParamterToConfigMap(cm *corev1.ConfigMap, prometheusService string) {
	processConfigMapsPrometheusService(cm, prometheusService) //ConfigMapData's PrometheusService
}

func processDeploymentPrometheusService(dep *appsv1.Deployment, prometheusservice string) {
	if flag, envIndex, ctnIndex := isPrometheusServiceDep(dep); flag == true && prometheusservice != "" {
		dep.Spec.Template.Spec.Containers[ctnIndex].Env[envIndex].Value = prometheusservice
	}
}

func isPrometheusServiceDep(dep *appsv1.Deployment) (bool, int, int) {
	for ctnIndex, v := range dep.Spec.Template.Spec.Containers {
		if len(v.Env) > 0 {
			for envIndex, value := range dep.Spec.Template.Spec.Containers[ctnIndex].Env {
				if value.Name == util.OriginDeploymentPrometheusLocation {
					return true, envIndex, ctnIndex
				}
			}
			return false, -1, -1
		}
	}
	return false, -1, -1
}

func GlobalSectionSetParamterToPersistentVolumeClaim(pvc *corev1.PersistentVolumeClaim, asp *alamedaserviceparamter.AlamedaServiceParamter) {
	for _, pvcusage := range v1alpha1.PvcUsage {
		if strings.Contains(pvc.Name, string(pvcusage)) {
			util.SetStorageToPersistentVolumeClaimSpec(pvc, asp.Storages, pvcusage)
		}
	}
}