package component

import (
	"bytes"
	"crypto"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"html/template"
	"math"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	autoscaling_v1alpha1 "github.com/containers-ai/alameda/operator/api/v1alpha1"
	federatoraiv1alpha1 "github.com/containers-ai/federatorai-operator/pkg/apis/federatorai/v1alpha1"
	"github.com/containers-ai/federatorai-operator/pkg/assets"
	"github.com/containers-ai/federatorai-operator/pkg/lib/resourceread"
	routev1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"

	admissionregistration_v1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	unstructuredv1 "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/cert"
	apiregistrationv1beta1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller_alamedaservice")

type ComponentConfig struct {
	Platform                 string
	NameSpace                string
	PodSecurityPolicyGroup   string
	PodSecurityPolicyVersion string

	PodTemplateConfig PodTemplateConfig

	FederatoraiAgentGPU FederatoraiAgentGPUConfig
	AIDispatcher        AIDispatcherConfig
	Execution           ExecutionConfig

	Image ImageConfig

	Prometheus PrometheusConfig
	Kafka      KafkaConfig
}

func NewComponentConfig(ptc PodTemplateConfig, alamedaService federatoraiv1alpha1.AlamedaService, opts ...ComponentConfigOption) *ComponentConfig {

	var (
		defaultNamespace                = ""
		defaultPodSecurityPolicyGroup   = policyv1beta1.SchemeGroupVersion.Group
		defaultPodSecurityPolicyVersion = policyv1beta1.SchemeGroupVersion.Version
		defaultPrometheusConfig         = PrometheusConfig{}
	)

	c := ComponentConfig{
		Platform:                 alamedaService.Spec.Platform,
		NameSpace:                defaultNamespace,
		PodSecurityPolicyGroup:   defaultPodSecurityPolicyGroup,
		PodSecurityPolicyVersion: defaultPodSecurityPolicyVersion,
		PodTemplateConfig:        ptc,
		FederatoraiAgentGPU:      NewDefaultFederatoraiAgentGPUConfig(),
		AIDispatcher:             NewDefaultAIDispatcherConfig(),
		Image:                    NewDefautlImageConfig(),
		Prometheus:               defaultPrometheusConfig,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return &c
}

func (c *ComponentConfig) SetNameSpace(ns string) {
	c.NameSpace = ns
}

func (c ComponentConfig) templateAssets(data string) []byte {
	tmpl, err := template.New("namespaceServiceToYaml").Parse(data)
	if err != nil {
		panic(err)
	}
	yamlBuffer := new(bytes.Buffer)
	if err = tmpl.Execute(yamlBuffer, c); err != nil {
		panic(err)
	}
	return yamlBuffer.Bytes()
}

func (c ComponentConfig) NewUnstructed(str string) (*unstructuredv1.Unstructured, error) {
	assetBytes, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Errorf("get asset bytes failed: %s", err.Error())
	}
	assetBytes = c.templateAssets(string(assetBytes[:]))
	assetJSONBytes, err := yaml.YAMLToJSON(assetBytes)
	if err != nil {
		return nil, errors.Errorf("get asset JSON bytes failed: %s", err.Error())
	}
	obj, err := resourceread.ReadJSONBytes(assetJSONBytes)
	if err != nil {
		return nil, errors.Errorf("get Unstructed failed: %s", err.Error())
	}
	return obj, nil
}

func (c ComponentConfig) NewClusterRoleBinding(str string) *rbacv1.ClusterRoleBinding {
	crbByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create clusterrolebinding")

	}
	crb := resourceread.ReadClusterRoleBindingV1(c.templateAssets(string(crbByte[:])))
	return crb
}

func (c ComponentConfig) NewClusterRole(str string) *rbacv1.ClusterRole {
	crByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create clusterrole")
	}
	cr := resourceread.ReadClusterRoleV1(c.templateAssets(string(crByte[:])))
	return cr
}

func (c ComponentConfig) NewRoleBinding(str string) *rbacv1.RoleBinding {
	crbByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create clusterrolebinding")

	}
	crb := resourceread.ReadRoleBindingV1(c.templateAssets(string(crbByte[:])))
	return crb
}

func (c ComponentConfig) NewRole(str string) *rbacv1.Role {
	crByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create clusterrole")
	}
	cr := resourceread.ReadRoleV1(c.templateAssets(string(crByte[:])))
	return cr
}

func (c ComponentConfig) NewPodSecurityPolicy(str string) (runtime.Object, error) {
	pspByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create PodSecurityPolicy")
	}
	var psp runtime.Object
	switch c.PodSecurityPolicyGroup {
	case policyv1beta1.SchemeGroupVersion.Group:
		policyV1beta1PodSecurityPolicy, err := resourceread.ReadPolicyV1beta1PodSecurityPolicy(c.templateAssets(string(pspByte[:])))
		if err != nil {
			return nil, err
		}
		psp = &policyV1beta1PodSecurityPolicy
	case extensionsv1beta1.SchemeGroupVersion.Group:
		psp = resourceread.ReadPodSecurityPolicyV1beta1(c.templateAssets(string(pspByte[:])))
	default:
		return nil, errors.Errorf(`not supported group "%s"`, c.PodSecurityPolicyGroup)
	}
	return psp, nil
}

func (c ComponentConfig) NewAlamedaNotificationChannel(str string) (*unstructuredv1.Unstructured, error) {
	assetBytes, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Errorf("get asset bytes failed: %s", err.Error())
	}
	assetBytes = c.templateAssets(string(assetBytes[:]))
	assetJSONBytes, err := yaml.YAMLToJSON(assetBytes)
	if err != nil {
		return nil, errors.Errorf("get asset JSON bytes failed: %s", err.Error())
	}
	obj, err := resourceread.ReadJSONBytes(assetJSONBytes)
	if err != nil {
		return nil, errors.Errorf("get AlamedaNotificationChannel failed: %s", err.Error())
	}
	return obj, nil
}

func (c ComponentConfig) NewAlamedaNotificationTopic(str string) (*unstructuredv1.Unstructured, error) {
	assetBytes, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Errorf("get asset bytes failed: %s", err.Error())
	}
	assetBytes = c.templateAssets(string(assetBytes[:]))
	assetJSONBytes, err := yaml.YAMLToJSON(assetBytes)
	if err != nil {
		return nil, errors.Errorf("get asset JSON bytes failed: %s", err.Error())
	}
	obj, err := resourceread.ReadJSONBytes(assetJSONBytes)
	if err != nil {
		return nil, errors.Errorf("get AlamedaNotificationTopic failed: %s", err.Error())
	}
	return obj, nil
}

func (c ComponentConfig) NewSecurityContextConstraints(str string) *securityv1.SecurityContextConstraints {
	sccByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create SecurityContextConstraints")
	}
	scc := resourceread.ReadSecurityContextConstraintsV1(c.templateAssets(string(sccByte[:])))
	return scc
}
func (c ComponentConfig) NewDaemonSet(str string) *appsv1.DaemonSet {
	daemonSetBytes, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create DaemonSet")

	}
	d := resourceread.ReadDaemonSetV1(c.templateAssets(string(daemonSetBytes[:])))
	d.Spec.Template = c.mutatePodTemplateSpecWithConfig(d.Spec.Template)
	return d
}
func (c ComponentConfig) NewServiceAccount(str string) *corev1.ServiceAccount {
	saByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create serviceaccount")

	}
	sa := resourceread.ReadServiceAccountV1(c.templateAssets(string(saByte[:])))
	return sa
}
func (c ComponentConfig) NewConfigMap(str string) *corev1.ConfigMap {
	cmByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create configmap")
	}

	var cm *corev1.ConfigMap
	cm = resourceread.ReadConfigMapV1(c.templateAssets(string(cmByte[:])))
	return cm
}
func (c ComponentConfig) NewPersistentVolumeClaim(str string) *corev1.PersistentVolumeClaim {
	pvcByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create persistentvolumeclaim")

	}
	pvc := resourceread.ReadPersistentVolumeClaimV1(c.templateAssets(string(pvcByte[:])))
	return pvc
}
func (c ComponentConfig) NewService(str string) *corev1.Service {
	svByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create service")

	}
	sv := resourceread.ReadServiceV1(c.templateAssets(string(svByte[:])))
	return sv
}

func (c ComponentConfig) NewAlamedaScaler(str string) *autoscaling_v1alpha1.AlamedaScaler {
	scalerByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create NewAlamedaScaler")

	}
	scaler := resourceread.ReadScalerV1(c.templateAssets(string(scalerByte[:])))
	return scaler
}

func (c ComponentConfig) NewDeployment(str string) *appsv1.Deployment {
	deploymentBytes, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create deployment")

	}
	d := resourceread.ReadDeploymentV1(c.templateAssets(string(deploymentBytes[:])))
	d.Spec.Template = c.mutatePodTemplateSpecWithConfig(d.Spec.Template)
	return d
}

func (c ComponentConfig) NewRoute(str string) *routev1.Route {
	rtByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create route")

	}
	rt := resourceread.ReadRouteV1(c.templateAssets(string(rtByte[:])))
	return rt
}

func (c ComponentConfig) NewIngress(str string) *extensionsv1beta1.Ingress {
	igByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create ingress")

	}
	ig := resourceread.ReadIngressv1beta1(c.templateAssets(string(igByte[:])))
	return ig
}

func (c ComponentConfig) NewStatefulSet(str string) *appsv1.StatefulSet {
	ssByte, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create statefulset")

	}
	ss := resourceread.ReadStatefulSetV1(c.templateAssets(string(ssByte[:])))
	return ss
}

func (c ComponentConfig) NewAdmissionControllerSecret() (*corev1.Secret, error) {

	secret, err := c.NewSecret("Secret/admission-controller-tls.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to buiild admission-controller secret")
	}

	caKey, err := NewPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "new ca private key failed")
	}

	caCertCfg := cert.Config{}
	caCert, err := cert.NewSelfSignedCACert(caCertCfg, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "new ca cert failed")
	}

	admctlKey, err := NewPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "new admctl private key failed")
	}

	admctlCertCfg := cert.Config{
		CommonName: fmt.Sprintf("admission-controller.%s.svc", c.NameSpace),
		Usages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}
	admctlCert, err := NewSignedCert(admctlCertCfg, admctlKey, caCert, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "new admctl cert failed")
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["ca.crt"] = EncodeCertPEM(caCert)
	secret.Data["tls.crt"] = EncodeCertPEM(admctlCert)
	secret.Data["tls.key"] = EncodePrivateKeyPEM(admctlKey)

	return secret, nil
}

func (c ComponentConfig) NewTLSSecret(assetFile, cn string) (*corev1.Secret, error) {

	secret, err := c.NewSecret(assetFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to buiild secret")
	}

	caKey, err := NewPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "new ca private key failed")
	}

	caCertCfg := cert.Config{}
	caCert, err := cert.NewSelfSignedCACert(caCertCfg, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "new ca cert failed")
	}

	privateKey, err := NewPrivateKey()
	if err != nil {
		return nil, errors.Wrap(err, "new private key failed")
	}

	certCfg := cert.Config{
		CommonName: cn,
		Usages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}
	certificate, err := NewSignedCert(certCfg, privateKey, caCert, caKey)
	if err != nil {
		return nil, errors.Wrap(err, "new certificate failed")
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["ca.crt"] = EncodeCertPEM(caCert)
	secret.Data["tls.crt"] = EncodeCertPEM(certificate)
	secret.Data["tls.key"] = EncodePrivateKeyPEM(privateKey)

	return secret, nil
}

func (c ComponentConfig) NewfedemeterSecret() (*corev1.Secret, error) {
	secret, err := c.NewSecret("Secret/fedemeter-tls.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to buiild fedemeter secret")
	}
	host := fmt.Sprintf("fedemeter-api.%s.svc", c.NameSpace)
	crt, key, err := cert.GenerateSelfSignedCertKey(host, []net.IP{}, []string{})
	if err != nil {
		return nil, errors.Errorf("failed to buiild fedemeter secret: %s", err.Error())
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["tls.crt"] = crt
	secret.Data["tls.key"] = key
	return secret, nil
}

func (c ComponentConfig) NewInfluxDBSecret() (*corev1.Secret, error) {

	secret, err := c.NewSecret("Secret/alameda-influxdb.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to buiild influxdb secret")
	}

	host := fmt.Sprintf("admission-influxdb.%s.svc", c.NameSpace)
	crt, key, err := cert.GenerateSelfSignedCertKey(host, []net.IP{}, []string{})
	if err != nil {
		return nil, errors.Errorf("failed to buiild influxdb secret: %s", err.Error())
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["tls.crt"] = crt
	secret.Data["tls.key"] = key

	return secret, nil
}

func (c ComponentConfig) NewSecret(str string) (*corev1.Secret, error) {
	secretBytes, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read secret from assets' bin data")
	}
	s, err := resourceread.ReadSecretV1(c.templateAssets(string(secretBytes[:])))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build secret from assets' bin data")
	}
	return s, nil
}

func (c ComponentConfig) NewMutatingWebhookConfiguration(str string) (*admissionregistration_v1beta1.MutatingWebhookConfiguration, error) {
	assetByte, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read MutatingWebhookConfiguration from assets' bin data")
	}
	resource, err := resourceread.ReadMutatingWebhookConfiguration(c.templateAssets(string(assetByte[:])))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build MutatingWebhookConfiguration from assets' bin data")
	}
	return resource, nil
}

func (c ComponentConfig) NewValidatingWebhookConfiguration(str string) (*admissionregistration_v1beta1.ValidatingWebhookConfiguration, error) {
	assetByte, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build ValidatingWebhookConfiguration from assets' bin data")
	}
	resource, err := resourceread.ReadValidatingWebhookConfiguration(c.templateAssets(string(assetByte[:])))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build ValidatingWebhookConfiguration from assets' bin data")
	}
	return resource, nil
}

func (c ComponentConfig) NewAPIService(str string) (*apiregistrationv1beta1.APIService, error) {
	assetByte, err := assets.Asset(str)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build APIService from assets' bin data")
	}
	resource, err := resourceread.ReadAPIService(c.templateAssets(string(assetByte[:])))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build APIService from assets' bin data")
	}
	return resource, nil
}

func (c ComponentConfig) NewCustomResourceDefinition(str string) *apiextv1beta1.CustomResourceDefinition {
	crdBytes, err := assets.Asset(str)
	if err != nil {
		log.Error(err, "Failed to Test create testcrd")
	}
	crd := resourceread.ReadCustomResourceDefinitionV1Beta1(c.templateAssets(string(crdBytes[:])))
	return crd
}

func (c ComponentConfig) mutatePodTemplateSpecWithConfig(podTemplateSpec corev1.PodTemplateSpec) corev1.PodTemplateSpec {

	copyPodTemplateSpec := podTemplateSpec.DeepCopy()

	var currentPodSecurityContext corev1.PodSecurityContext
	if copyPodTemplateSpec.Spec.SecurityContext != nil {
		currentPodSecurityContext = *copyPodTemplateSpec.Spec.SecurityContext
	}
	podSecurityContext := c.mutatePodSecurityContextWithConfig(currentPodSecurityContext)
	copyPodTemplateSpec.Spec.SecurityContext = &podSecurityContext

	return *copyPodTemplateSpec
}

func (c ComponentConfig) mutatePodSecurityContextWithConfig(podSecurityContext corev1.PodSecurityContext) corev1.PodSecurityContext {

	copyPodSecurityContext := podSecurityContext.DeepCopy()

	if c.PodTemplateConfig.PodSecurityContext.FSGroup != nil {
		fsGroup := *c.PodTemplateConfig.PodSecurityContext.FSGroup
		copyPodSecurityContext.FSGroup = &fsGroup
	}

	if c.PodTemplateConfig.PodSecurityContext.RunAsUser != nil {
		runAsUser := *c.PodTemplateConfig.PodSecurityContext.RunAsUser
		copyPodSecurityContext.RunAsUser = &runAsUser
	}

	if c.PodTemplateConfig.PodSecurityContext.RunAsGroup != nil {
		runAsGroup := *c.PodTemplateConfig.PodSecurityContext.RunAsGroup
		copyPodSecurityContext.RunAsGroup = &runAsGroup
	}

	if c.PodTemplateConfig.PodSecurityContext.SELinuxOptions != nil {
		seLinuxOptions := *c.PodTemplateConfig.PodSecurityContext.SELinuxOptions
		copyPodSecurityContext.SELinuxOptions = &seLinuxOptions
	}

	if c.PodTemplateConfig.PodSecurityContext.SupplementalGroups != nil {
		supplementalGroups := c.PodTemplateConfig.PodSecurityContext.SupplementalGroups
		copyPodSecurityContext.SupplementalGroups = supplementalGroups
	}

	if c.PodTemplateConfig.PodSecurityContext.Sysctls != nil {
		sysctls := c.PodTemplateConfig.PodSecurityContext.Sysctls
		copyPodSecurityContext.Sysctls = sysctls
	}

	return *copyPodSecurityContext
}

// PodTemplateConfig specifies pod confiruation needed while deploying pod
type PodTemplateConfig struct {
	corev1.PodSecurityContext
}

func NewDefaultPodTemplateConfig(ns corev1.Namespace) PodTemplateConfig {

	var (
		ptc PodTemplateConfig

		defaultPSC         corev1.PodSecurityContext
		okdPreAllocatedPSC corev1.PodSecurityContext
	)

	defaultPSC = newDefaultPodSecurityContext()
	ptc = PodTemplateConfig{
		PodSecurityContext: defaultPSC,
	}

	okdPreAllocatedPSC = newOKDPreAllocatedPodSecurityContext(ns)
	ptc.PodSecurityContext = overwritePodSecurityContextFromOKDPodSecurityContext(ptc.PodSecurityContext, okdPreAllocatedPSC)

	return ptc
}

func newDefaultPodSecurityContext() corev1.PodSecurityContext {

	var (
		defaultFSGroup = int64(1000)
	)

	psc := corev1.PodSecurityContext{
		FSGroup: &defaultFSGroup,
	}

	return psc
}

// Currently implement fsGroup strategy.
// Please reference okd documentation https://docs.okd.io/latest/architecture/additional_concepts/authorization.html#understanding-pre-allocated-values-and-security-context-constraints
func newOKDPreAllocatedPodSecurityContext(ns corev1.Namespace) corev1.PodSecurityContext {

	var psc corev1.PodSecurityContext

	annotations := ns.GetObjectMeta().GetAnnotations()

	var fsGroup *int64
	minFSGroupValueString := ""
	if fsGroupRanges, exist := annotations["openshift.io/sa.scc.supplemental-groups"]; exist {
		firstFSGroupRange := strings.Split(fsGroupRanges, ",")[0]
		if strings.Contains(firstFSGroupRange, "/") {
			minFSGroupValueString = strings.Split(firstFSGroupRange, "/")[0]
		} else if strings.Contains(firstFSGroupRange, "-") {
			minFSGroupValueString = strings.Split(firstFSGroupRange, "-")[0]
		}
	} else if fsGroupRange, exist := annotations["openshift.io/sa.scc.uid-range"]; exist {
		if strings.Contains(fsGroupRange, "/") {
			minFSGroupValueString = strings.Split(fsGroupRange, "/")[0]
		}
	}
	if minFSGroupValueString != "" {
		if minFSGroupValue, err := strconv.ParseInt(minFSGroupValueString, 10, 64); err != nil {
			log.V(-1).Info("parse minimum fsGroup value from namespace's annotation failed", "errMsg", err.Error())
		} else {
			fsGroup = &minFSGroupValue
		}
	}
	psc.FSGroup = fsGroup

	return psc
}

// Currently overwrite fsGroup
// Please reference okd documentation https://docs.okd.io/latest/architecture/additional_concepts/authorization.html#understanding-pre-allocated-values-and-security-context-constraints
func overwritePodSecurityContextFromOKDPodSecurityContext(psc, okdPSC corev1.PodSecurityContext) corev1.PodSecurityContext {

	copyPSC := psc.DeepCopy()
	copyOKDPSC := okdPSC.DeepCopy()

	copyPSC.FSGroup = copyOKDPSC.FSGroup

	return *copyPSC
}

const (
	rsaKeySize             = 2048
	duration365d           = time.Hour * 24 * 365
	CertificateBlockType   = "CERTIFICATE"
	RSAPrivateKeyBlockType = "RSA PRIVATE KEY"
)

func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
}

func NewSignedCert(cfg cert.Config, key crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer) (*x509.Certificate, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     time.Now().Add(duration365d).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func EncodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  CertificateBlockType,
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}
