package alamedaservicekeycode

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	alamedautilsk8s "github.com/containers-ai/alameda/pkg/utils/kubernetes"
	datahubv1alpha1_event "github.com/containers-ai/api/alameda_api/v1alpha1/datahub/events"
	federatoraiv1alpha1 "github.com/containers-ai/federatorai-operator/pkg/apis/federatorai/v1alpha1"
	client_datahub "github.com/containers-ai/federatorai-operator/pkg/client/datahub"
	"github.com/containers-ai/federatorai-operator/pkg/component"
	"github.com/containers-ai/federatorai-operator/pkg/processcrdspec/alamedaserviceparamter"
	repository_keycode "github.com/containers-ai/federatorai-operator/pkg/repository/keycode"
	repository_keycode_datahub "github.com/containers-ai/federatorai-operator/pkg/repository/keycode/datahub"
	"github.com/containers-ai/federatorai-operator/pkg/util"

	"github.com/pkg/errors"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type namespace = string

const (
	retryLimitDuration                  time.Duration = 30 * time.Minute
	addKeycodeSuccessMessageTemplate                  = "Add keycode %s success"
	deleteKeycodeSuccessMessageTemplate               = "Delete keycode %s success"
	addKeycodeFailedMessageTemplate                   = "Add keycode %s failed"
	deleteKeycodeFailedMessageTemplate                = "Delete keycode %s failed"
)

var (
	_                   reconcile.Reconciler = &ReconcileAlamedaServiceKeycode{}
	log                                      = logf.Log.WithName("controller_alamedaservicekeycode")
	requeueDuration                          = 30 * time.Second
	keycodeSpecialCases                      = []string{
		"GRV7JLA4TXKPPITS6GRSNK4EBILFRQ",
	}
)

// Add creates a new AlamedaService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {

	var clusterID string
	tmpClient, err := client.New(mgr.GetConfig(), client.Options{})
	if err != nil {
		log.V(-1).Info("Get tmp client failed, will use \"\" as clusterID when sending event.", "error", err.Error())
	} else {
		clusterID, err = alamedautilsk8s.GetClusterUID(tmpClient)
		if err != nil {
			log.V(-1).Info("Get clusterID failed, will use \"\" as clusterID when sending event.", "error", err.Error())
		}
	}

	return &ReconcileAlamedaServiceKeycode{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),

		datahubClientMap:     make(map[namespace]client_datahub.Client),
		datahubClientMapLock: sync.Mutex{},

		firstRetryTimeCache: make(map[types.NamespacedName]*time.Time),
		firstRetryTimeLock:  sync.Mutex{},

		clusterID:        clusterID,
		eventChanMap:     make(map[namespace]chan datahubv1alpha1_event.Event),
		eventChanMapLock: sync.Mutex{},

		lastReconcileTaskMap: make(map[namespace]struct {
			codeNumber string
			state      federatoraiv1alpha1.KeycodeState
		}),
		lastReconcileTaskMapLock: sync.Mutex{},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("alamedaservicekeycode-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Watch for changes to primary resource AlamedaService
	err = c.Watch(&source.Kind{Type: &federatoraiv1alpha1.AlamedaService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

// ReconcileAlamedaServiceKeycode reconciles a AlamedaService object
type ReconcileAlamedaServiceKeycode struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	datahubClientMap     map[namespace]client_datahub.Client
	datahubClientMapLock sync.Mutex

	firstRetryTimeCache map[types.NamespacedName]*time.Time
	firstRetryTimeLock  sync.Mutex

	clusterID        string
	eventChanMap     map[namespace]chan datahubv1alpha1_event.Event
	eventChanMapLock sync.Mutex

	lastReconcileTaskMap map[namespace]struct {
		codeNumber string
		state      federatoraiv1alpha1.KeycodeState
	}
	lastReconcileTaskMapLock sync.Mutex
}

// Reconcile reconcile AlamedaService's keycode
func (r *ReconcileAlamedaServiceKeycode) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	log.Info("Reconcile Keycode")

	var reconcileResult = reconcile.Result{}
	alamedaService := &federatoraiv1alpha1.AlamedaService{}
	defer r.setLastReconcileTask(request.Namespace, alamedaService)
	defer r.flushEvents(request.Namespace, alamedaService)
	defer r.handleFirstRetryTime(&reconcileResult, request.NamespacedName)
	defer func() {

		instance := &federatoraiv1alpha1.AlamedaService{}
		err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: request.Namespace, Name: request.Name}, instance)
		if err != nil && k8sErrors.IsNotFound(err) {
			addr, err := r.getDatahubAddressByNamespace(request.Namespace)
			if err != nil {
				log.V(-1).Info("Get datahub address failed, skip deleting datahub client", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
			}
			err = r.deleteDatahubClient(addr)
			if err != nil {
				log.V(-1).Info("Deleting datahub client failed", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
			}
			return
		} else if err != nil {
			log.V(-1).Info("Get AlamedaService failed, skip writing status", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
			return
		}

		instance.Spec.Keycode = alamedaService.Spec.Keycode
		instance.Status.KeycodeStatus = alamedaService.Status.KeycodeStatus

		// Get keycodeRepository
		keycodeRepository, err := r.getKeycodeRepository(request.Namespace)
		if err != nil {
			log.V(-1).Info("Get keycode summary failed, will not write keycode summary into AlamedaService's status", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		} else {
			detail, err := keycodeRepository.GetKeycodeDetail("")
			if err != nil {
				log.V(-1).Info("Get keycode summary failed, write empty keycode summary into AlamedaService's status", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
				reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			}
			instance.SetStatusKeycodeSummary(detail.Summary())
		}

		if err := r.client.Update(context.Background(), instance); err != nil {
			log.V(-1).Info("Update AlamedaService status failed", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name, "error", err.Error())
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		}
	}()

	// Fetch the AlamedaService instance
	err := r.client.Get(context.TODO(), request.NamespacedName, alamedaService)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			if err := r.deleteAlamedaServiceDependencies(alamedaService); err != nil {
				log.V(-1).Info("Handle AlamedaService deletion failed", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			}
			log.Info("AlamedaService not found, skip keycode reconciling", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name)
			reconcileResult.Requeue = false
			return reconcileResult, nil
		}
		// Error reading the object - requeue the request.
		log.V(-1).Info("Get AlamedaService failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
		reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		return reconcileResult, nil
	}

	if firstRetryTime := r.getFirstRetryTime(request.NamespacedName); firstRetryTime != nil {
		now := time.Now()
		if now.Sub(*firstRetryTime) > retryLimitDuration {
			log.Error(nil, "Exceeds retry limit, stop reconciing.", "AlamedaService.Namespace", request.Namespace, "AlamedaService.Name", request.Name)
			reconcileResult.Requeue = false
			return reconcileResult, nil
		}
	}

	// Get keycodeRepository
	keycodeRepository, err := r.getKeycodeRepository(alamedaService.Namespace)
	if err != nil {
		log.V(-1).Info("Get licese repository failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
		alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "get keycode repository instance failed").Error()
		reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		return reconcileResult, nil
	}

	// There are two conditions to handle,
	// first, keycode is empty
	// seconde, keycode is not empty
	if alamedaService.IsCodeNumberEmpty() {
		if err := r.handleEmptyKeycode(keycodeRepository, alamedaService); err != nil {
			log.V(-1).Info("Handle empty keycode failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "handle empty keycode failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}
		alamedaService.Spec.Keycode = federatoraiv1alpha1.KeycodeSpec{}
		alamedaService.Status.KeycodeStatus = federatoraiv1alpha1.KeycodeStatus{State: federatoraiv1alpha1.KeycodeStateWaitingKeycode}
		log.Info("Handle empty keycode done", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		return reconcileResult, nil
	}

	// Check if need to reconcile keycode
	if r.needToReconcile(alamedaService) {
		log.Info("Keycode not changed, skip reconciling", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		return reconcileResult, nil
	}

	// If keycode is updated, do the update process no matter what the current state is
	if alamedaService.IsCodeNumberUpdated() {
		if err := r.handleKeycode(keycodeRepository, alamedaService); err != nil {
			log.V(-1).Info("Update keycode failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "update keycode failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}
		log.Info("Update keycode done, start polling registration data", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		return reconcileResult, nil
	}

	// Keycode is not changed, process keycode by the current state
	switch alamedaService.Status.KeycodeStatus.State {
	case federatoraiv1alpha1.KeycodeStateDefault, federatoraiv1alpha1.KeycodeStateWaitingKeycode:
		if err := r.handleKeycode(keycodeRepository, alamedaService); err != nil {
			log.V(-1).Info("Handling keycode failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "handle keycode failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}
		log.Info("Handling keycode done, start polling registration data", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
		return reconcileResult, nil
	case federatoraiv1alpha1.KeycodeStatePollingRegistrationData:
		// This state will move to "federatoraiv1alpha1.KeycodeStateDone" state if the keycode detail is registered

		// Poll registration data from keycode repository
		registrationData, err := keycodeRepository.GetRegistrationData()
		if err != nil {
			log.V(-1).Info("Polling registration data failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "poll registration data failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}

		// Get keycode defailt from keycode repository
		detail, err := keycodeRepository.GetKeycodeDetail("")
		if err != nil {
			log.V(-1).Info("Polling registration data failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "poll registration data failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}
		if detail.Registered {
			alamedaService.Spec.Keycode.SignatureData = registrationData
			alamedaService.Status.KeycodeStatus = federatoraiv1alpha1.KeycodeStatus{
				CodeNumber:       alamedaService.Spec.Keycode.CodeNumber,
				RegistrationData: "",
				State:            federatoraiv1alpha1.KeycodeStateDone,
				LastErrorMessage: "",
				Summary:          "",
			}
			log.Info("Keycode has been registered, move state to \"Done\"", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		} else {
			alamedaService.Status.KeycodeStatus = federatoraiv1alpha1.KeycodeStatus{
				CodeNumber:       alamedaService.Spec.Keycode.CodeNumber,
				RegistrationData: registrationData,
				State:            federatoraiv1alpha1.KeycodeStateWaitingSignatureData,
				LastErrorMessage: "",
				Summary:          "",
			}
			log.Info("Polling registration data done, waiting signature data", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		}
		return reconcileResult, nil
	case federatoraiv1alpha1.KeycodeStateWaitingSignatureData:
		if alamedaService.Spec.Keycode.SignatureData == "" {
			log.Info("Waiting signature data to be filled in, skip reconciling", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
			return reconcileResult, nil
		}
		if err := r.handleSignatureData(keycodeRepository, alamedaService); err != nil {
			log.V(-1).Info("Handling signature data failed, retry reconciling keycode", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "error", err.Error())
			alamedaService.Status.KeycodeStatus.LastErrorMessage = errors.Wrap(err, "handle signature data  failed").Error()
			reconcileResult = reconcile.Result{Requeue: true, RequeueAfter: requeueDuration}
			return reconcileResult, nil
		}
		alamedaService.Status.KeycodeStatus.LastErrorMessage = ""
		alamedaService.Status.KeycodeStatus.State = federatoraiv1alpha1.KeycodeStateDone
		log.Info("Handling signature data done", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name)
		return reconcileResult, nil
	default:
		log.Info("Unknown keycode state, skip reconciling", "AlamedaService.Namespace", alamedaService.Namespace, "AlamedaService.Name", alamedaService.Name, "state", alamedaService.Status.KeycodeStatus.State)
		return reconcileResult, nil
	}
}

// handleFirstRetryTime set/resets first retry time when requeue is true/false
func (r *ReconcileAlamedaServiceKeycode) handleFirstRetryTime(reconcileResult *reconcile.Result, namespacedName types.NamespacedName) {

	if reconcileResult.Requeue == true {
		t := time.Now()
		r.setFirstRetryTimeIfNotExist(namespacedName, &t)
	} else {
		r.setFirstRetryTime(namespacedName, nil)
	}
}

func (r *ReconcileAlamedaServiceKeycode) deleteAlamedaServiceDependencies(alamedaService *federatoraiv1alpha1.AlamedaService) error {

	datahubAddress, err := r.getDatahubAddressByNamespace(alamedaService.Namespace)
	if err != nil {
		return errors.Wrap(err, "get datahub address failed")
	}
	r.deleteDatahubClient(datahubAddress)

	r.deleteFirstRetryTime(types.NamespacedName{Namespace: alamedaService.Namespace, Name: alamedaService.Name})
	r.deleteEventChan(alamedaService.Namespace)
	r.deleteLastReconcileTask(alamedaService.Namespace)

	return nil
}

func (r *ReconcileAlamedaServiceKeycode) deleteDatahubClient(datahubAddr string) error {

	if client, exist := r.datahubClientMap[datahubAddr]; exist {
		r.datahubClientMapLock.Lock()
		defer r.datahubClientMapLock.Unlock()
		if err := client.Close(); err != nil {
			return err
		}
		delete(r.datahubClientMap, datahubAddr)
	}

	return nil
}

func (r *ReconcileAlamedaServiceKeycode) needToReconcile(alamedaService *federatoraiv1alpha1.AlamedaService) bool {
	return !alamedaService.IsCodeNumberUpdated() &&
		alamedaService.Status.KeycodeStatus.State == federatoraiv1alpha1.KeycodeStateDone
}

func (r *ReconcileAlamedaServiceKeycode) handleEmptyKeycode(keycodeRepository repository_keycode.Interface, alamedaService *federatoraiv1alpha1.AlamedaService) error {

	if !alamedaService.IsCodeNumberUpdated() {
		return nil
	}

	details, err := keycodeRepository.ListKeycodes()
	if err != nil {
		return errors.Wrap(err, "list keycodes failed")
	}
	for _, detail := range details {
		codeNumber := detail.Keycode
		if codeNumber == "" {
			continue
		}
		if err := keycodeRepository.DeleteKeycode(codeNumber); err != nil {
			e := newLicenseEvent(
				alamedaService.Namespace,
				fmt.Sprintf(deleteKeycodeFailedMessageTemplate, codeNumber),
				r.clusterID,
				datahubv1alpha1_event.EventLevel_EVENT_LEVEL_WARNING)
			r.addEvent(alamedaService.Namespace, e)
			return errors.Wrap(err, "delete keycode failed")
		}
		e := newLicenseEvent(
			alamedaService.Namespace,
			fmt.Sprintf(deleteKeycodeSuccessMessageTemplate, codeNumber),
			r.clusterID,
			datahubv1alpha1_event.EventLevel_EVENT_LEVEL_INFO)
		r.addEvent(alamedaService.Namespace, e)
	}

	return nil
}

func (r *ReconcileAlamedaServiceKeycode) handleKeycode(keycodeRepository repository_keycode.Interface, alamedaService *federatoraiv1alpha1.AlamedaService) error {

	// Check if keycode is existing
	keycode := alamedaService.Spec.Keycode.CodeNumber
	details, err := keycodeRepository.ListKeycodes()
	if err != nil {
		return errors.Wrap(err, "list keycodes failed")
	}

	if len(details) == 0 {
		// Apply keycode to keycode repository
		if err := keycodeRepository.SendKeycode(keycode); err != nil {
			e := newLicenseEvent(
				alamedaService.Namespace,
				fmt.Sprintf(addKeycodeFailedMessageTemplate, keycode),
				r.clusterID,
				datahubv1alpha1_event.EventLevel_EVENT_LEVEL_WARNING)
			r.addEvent(alamedaService.Namespace, e)
			return errors.Wrap(err, "send keycode to keycode repository failed")
		}
		e := newLicenseEvent(
			alamedaService.Namespace,
			fmt.Sprintf(addKeycodeSuccessMessageTemplate, keycode),
			r.clusterID,
			datahubv1alpha1_event.EventLevel_EVENT_LEVEL_INFO)
		r.addEvent(alamedaService.Namespace, e)
		alamedaService.Status.KeycodeStatus.CodeNumber = alamedaService.Spec.Keycode.CodeNumber
		alamedaService.Status.KeycodeStatus.State = federatoraiv1alpha1.KeycodeStatePollingRegistrationData
	} else {
		// If keycode is special case, get the existing keycode for user
		if r.isKeycodeSpecialCase(keycode) {
			alamedaService.Spec.Keycode.CodeNumber = details[0].Keycode
			alamedaService.Spec.Keycode.SignatureData = ""
			alamedaService.Status.KeycodeStatus.CodeNumber = details[0].Keycode
			alamedaService.Status.KeycodeStatus.RegistrationData = ""
			alamedaService.Status.KeycodeStatus.State = federatoraiv1alpha1.KeycodeStatePollingRegistrationData
			alamedaService.Status.KeycodeStatus.LastErrorMessage = ""
			alamedaService.Status.KeycodeStatus.Summary = ""
		} else {

			for _, detail := range details {
				codeNumber := detail.Keycode
				if codeNumber == "" {
					continue
				}
				if err := keycodeRepository.DeleteKeycode(codeNumber); err != nil {
					e := newLicenseEvent(
						alamedaService.Namespace,
						fmt.Sprintf(deleteKeycodeFailedMessageTemplate, keycode),
						r.clusterID,
						datahubv1alpha1_event.EventLevel_EVENT_LEVEL_WARNING)
					r.addEvent(alamedaService.Namespace, e)
					return errors.Wrap(err, "delete keycode failed")
				}
				e := newLicenseEvent(
					alamedaService.Namespace,
					fmt.Sprintf(deleteKeycodeSuccessMessageTemplate, codeNumber),
					r.clusterID,
					datahubv1alpha1_event.EventLevel_EVENT_LEVEL_INFO)
				r.addEvent(alamedaService.Namespace, e)
			}

			alamedaService.Spec.Keycode.SignatureData = ""
			alamedaService.Status.KeycodeStatus.CodeNumber = ""
			alamedaService.Status.KeycodeStatus.RegistrationData = ""
			alamedaService.Status.KeycodeStatus.State = federatoraiv1alpha1.KeycodeStateWaitingKeycode
			alamedaService.Status.KeycodeStatus.LastErrorMessage = ""
			alamedaService.Status.KeycodeStatus.Summary = ""

			// Apply keycode to keycode repository
			if err := keycodeRepository.SendKeycode(keycode); err != nil {
				return errors.Wrap(err, "send keycode to keycode repository failed")
			}
			e := newLicenseEvent(
				alamedaService.Namespace,
				fmt.Sprintf(addKeycodeSuccessMessageTemplate, keycode),
				r.clusterID,
				datahubv1alpha1_event.EventLevel_EVENT_LEVEL_INFO)
			r.addEvent(alamedaService.Namespace, e)
			alamedaService.Status.KeycodeStatus.CodeNumber = alamedaService.Spec.Keycode.CodeNumber
			alamedaService.Status.KeycodeStatus.State = federatoraiv1alpha1.KeycodeStatePollingRegistrationData
		}
	}

	return nil
}

func (r *ReconcileAlamedaServiceKeycode) isKeycodeSpecialCase(keycode string) bool {

	for _, c := range keycodeSpecialCases {
		keycode = strings.Replace(keycode, "-", "", -1)
		if c == keycode {
			return true
		}
	}

	return false
}

func (r *ReconcileAlamedaServiceKeycode) handleSignatureData(keycodeRepository repository_keycode.Interface, alamedaService *federatoraiv1alpha1.AlamedaService) error {

	// Sending registration data to keycode repository
	err := keycodeRepository.SendSignatureData(alamedaService.Spec.Keycode.SignatureData)
	if err != nil {
		return errors.Wrap(err, "send signature data to keycode repository failed")
	}

	return nil
}

func (r *ReconcileAlamedaServiceKeycode) getKeycodeRepository(namespace string) (repository_keycode.Interface, error) {

	datahubAddress, err := r.getDatahubAddressByNamespace(namespace)
	if err != nil {
		return nil, errors.Wrap(err, "get Datahub address failed")
	}
	datahubClient := r.getOrCreateDatahubClient(datahubAddress)
	keycodeRepository := repository_keycode_datahub.NewKeycodeRepository(&datahubClient)

	return keycodeRepository, nil
}

func (r *ReconcileAlamedaServiceKeycode) getDatahubAddressByNamespace(namespace string) (string, error) {

	componentFactory := component.ComponentConfig{NameSpace: namespace}

	// Get datahub client instance
	datahubServiceAssetName := alamedaserviceparamter.GetAlamedaDatahubService()
	datahubService := componentFactory.NewService(datahubServiceAssetName)
	datahubAddress, err := util.GetServiceAddress(datahubService, "grpc")
	if err != nil {
		return "", err
	}
	return datahubAddress, nil
}

func (r *ReconcileAlamedaServiceKeycode) getOrCreateDatahubClient(datahubAddress string) client_datahub.Client {

	if _, exist := r.datahubClientMap[datahubAddress]; !exist {
		r.datahubClientMapLock.Lock()
		defer r.datahubClientMapLock.Unlock()
		datahubClientConfig := client_datahub.NewDefaultConfig()
		datahubClientConfig.Address = datahubAddress
		r.datahubClientMap[datahubAddress] = client_datahub.NewDatahubClient(datahubClientConfig)
	}
	return r.datahubClientMap[datahubAddress]
}

func (r *ReconcileAlamedaServiceKeycode) setFirstRetryTimeIfNotExist(namespacedName types.NamespacedName, t *time.Time) {
	if r.getFirstRetryTime(namespacedName) == nil {
		r.setFirstRetryTime(namespacedName, t)
	}
}

func (r *ReconcileAlamedaServiceKeycode) setFirstRetryTime(namespacedName types.NamespacedName, t *time.Time) {

	r.firstRetryTimeLock.Lock()
	defer r.firstRetryTimeLock.Unlock()
	r.firstRetryTimeCache[namespacedName] = t
}

func (r *ReconcileAlamedaServiceKeycode) getFirstRetryTime(namespacedName types.NamespacedName) *time.Time {

	r.firstRetryTimeLock.Lock()
	defer r.firstRetryTimeLock.Unlock()
	return r.firstRetryTimeCache[namespacedName]
}

func (r *ReconcileAlamedaServiceKeycode) deleteFirstRetryTime(namespacedName types.NamespacedName) {

	r.firstRetryTimeLock.Lock()
	defer r.firstRetryTimeLock.Unlock()
	delete(r.firstRetryTimeCache, namespacedName)
}

func (r *ReconcileAlamedaServiceKeycode) getEventChan(namespace namespace) chan datahubv1alpha1_event.Event {

	var eventChan chan datahubv1alpha1_event.Event
	var exist bool
	if eventChan, exist = r.eventChanMap[namespace]; !exist {
		r.eventChanMapLock.Lock()
		r.eventChanMap[namespace] = make(chan datahubv1alpha1_event.Event, 100)
		r.eventChanMapLock.Unlock()
	}
	eventChan = r.eventChanMap[namespace]
	return eventChan
}

func (r *ReconcileAlamedaServiceKeycode) deleteEventChan(namespace namespace) {

	r.eventChanMapLock.Lock()
	defer r.eventChanMapLock.Unlock()
	delete(r.eventChanMap, namespace)
}

func (r *ReconcileAlamedaServiceKeycode) addEvent(namespace namespace, e datahubv1alpha1_event.Event) {

	var eventChan chan datahubv1alpha1_event.Event
	var exist bool
	if eventChan, exist = r.eventChanMap[namespace]; !exist {
		r.eventChanMapLock.Lock()
		r.eventChanMap[namespace] = make(chan datahubv1alpha1_event.Event, 100)
		eventChan = r.eventChanMap[namespace]
		r.eventChanMapLock.Unlock()
	}

	eventChan <- e
}

func (r *ReconcileAlamedaServiceKeycode) flushEvents(namespace namespace, alamedaService *federatoraiv1alpha1.AlamedaService) error {

	log.V(1).Info("Flush events...")

	datahubAddress, err := r.getDatahubAddressByNamespace(namespace)
	if err != nil {
		log.V(-1).Info("Flush events failed: get datahub address failed %s", err.Error())
	}

	cli := r.getOrCreateDatahubClient(datahubAddress)

	var events []*datahubv1alpha1_event.Event
	eventChan := r.getEventChan(namespace)
Loop:
	for {
		select {
		case event := <-eventChan:
			copyEvent := event
			events = append(events, &copyEvent)
		default:
			break Loop
		}
	}

	if !r.needToflushEvents(namespace, alamedaService) {
		log.V(1).Info("Need not to flush events")
		return nil
	}
	err = cli.CreateEvents(events)
	if err != nil {
		log.V(-1).Info("Flush events failed: %s", "error", err.Error())
	}

	log.V(1).Info("Flush events done")
	return nil
}

func (r *ReconcileAlamedaServiceKeycode) needToflushEvents(namespace namespace, alamedaService *federatoraiv1alpha1.AlamedaService) bool {

	if alamedaService == nil || alamedaService.DeletionTimestamp != nil {
		return true
	}

	lastReconcileTask := r.getLastReconcileTask(namespace)
	if lastReconcileTask.codeNumber == alamedaService.Spec.Keycode.CodeNumber &&
		lastReconcileTask.state == alamedaService.Status.KeycodeStatus.State {
		return false
	}

	return true
}

func (r *ReconcileAlamedaServiceKeycode) setLastReconcileTask(namespace namespace, alamedaService *federatoraiv1alpha1.AlamedaService) {
	if alamedaService == nil || alamedaService.DeletionTimestamp != nil {
		return
	}
	r.lastReconcileTaskMapLock.Lock()
	defer r.lastReconcileTaskMapLock.Unlock()
	r.lastReconcileTaskMap[namespace] = struct {
		codeNumber string
		state      federatoraiv1alpha1.KeycodeState
	}{
		codeNumber: alamedaService.Spec.Keycode.CodeNumber,
		state:      alamedaService.Status.KeycodeStatus.State,
	}
}

func (r *ReconcileAlamedaServiceKeycode) getLastReconcileTask(namespace namespace) struct {
	codeNumber string
	state      federatoraiv1alpha1.KeycodeState
} {
	return r.lastReconcileTaskMap[namespace]
}

func (r *ReconcileAlamedaServiceKeycode) deleteLastReconcileTask(namespace namespace) {

	r.lastReconcileTaskMapLock.Lock()
	defer r.lastReconcileTaskMapLock.Unlock()
	delete(r.lastReconcileTaskMap, namespace)
}
