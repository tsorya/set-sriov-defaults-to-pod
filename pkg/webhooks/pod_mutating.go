package webhook

import (
	"encoding/json"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"

	log "github.com/sirupsen/logrus"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	group      = "admission.setsriovdefaultpodannotation.openshift.io"
	resource   = "setsriovdefaultpodannotations"
	singularName   = "setsriovdefaultpodannotation"
	version    = "v1"
	defaultKey = "v1.multus-cni.io/default-network"
)

// PodSRIOVMutatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type PodSRIOVMutatingAdmissionHook struct {
	decoder *admission.Decoder
}

// NewAgentClusterInstallMutatingAdmissionHook constructs a new AgentClusterInstallMutatingAdmissionHook
func NewPodSRIOVMutatingAdmissionHook(decoder *admission.Decoder) *PodSRIOVMutatingAdmissionHook {
	return &PodSRIOVMutatingAdmissionHook{decoder: decoder}
}

// MutatingResource is the resource to use for hosting your admission webhook. (see https://github.com/openshift/generic-admission-server)
// The generic-admission-server uses the data below to register this webhook so when kube apiserver calls the REST path
// "/apis/admission.agentinstall.openshift.io/v1/agentclusterinstallmutators" the generic-admission-server calls
// the Admit() method below.
func (a *PodSRIOVMutatingAdmissionHook) MutatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    group,
		"resource": resource,
	}).Info("Registering mutating REST resource")
	// NOTE: This GVR is meant to be different than the AgentClusterInstall CRD GVR which has group "hivextension.openshift.io".

	return schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: resource,
		},
		singularName
}

// Initialize implements the AdmissionHook API. (see https://github.com/openshift/generic-admission-server)
// This function is called by generic-admission-server on startup to setup any special initialization
// that your webhook needs.
func (a *PodSRIOVMutatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    group,
		"resource": resource,
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Admit is called to decide whether to accept the admission request. The returned AdmissionResponse may
// use the Patch field to mutate the object from the passed AdmissionRequest. It implements the MutatingAdmissionHookV1
// interface. (see https://github.com/openshift/generic-admission-server)
func (a *PodSRIOVMutatingAdmissionHook) Admit(admissionSpec *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "Admit",
	})

	if !shouldValidate(admissionSpec) {
		contextLogger.Info("Skipping mutation for request")
		// The request object isn't something that this mutator should validate.
		// Therefore, we say that it's allowed.
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	contextLogger.Info("Mutating request")
	if admissionSpec.Operation == admissionv1.Create {
		return a.SetDefaults(admissionSpec)
	}

	// all other operations are explicitly allowed
	contextLogger.Info("No changes were made")
	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

func (a *PodSRIOVMutatingAdmissionHook) SetDefaults(admissionSpec *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "SetDefaults",
	})

	newObject := &corev1.Pod{}
	if err := a.decoder.DecodeRaw(admissionSpec.Object, newObject); err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	if !shouldSetDefault(newObject) {
		contextLogger.Info("Should not set default for pod %s", newObject.Name)
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	// Add the new data to the contextLogger
	contextLogger.Data["object.Name"] = newObject.Name

	mutated, _ := mutate(newObject)
	contextLogger.Debugf("Mutated object %v", mutated)
	patch, err := patch(admissionSpec.Object, mutated)
	contextLogger.Infof("Patch object %s", string(patch))
	patchType := admissionv1.PatchTypeJSONPatch
	if err != nil {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patch,
		PatchType: &patchType,
	}
}

func mutate(in *corev1.Pod) (out *corev1.Pod, err error) {
	current := in.DeepCopy()
	current.ObjectMeta.Annotations[defaultKey] = "default/default"
	return current, nil
}

func patch(original runtime.RawExtension, mutated *corev1.Pod) ([]byte, error) {
	current, err := json.Marshal(mutated)
	if err != nil {
		return nil, err
	}

	operations, err := jsonpatch.CreatePatch(original.Raw, current)
	if err != nil {
		return nil, err
	}

	return json.Marshal(operations)
}

// shouldValidate explicitly checks if the request should be validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func shouldValidate(admissionSpec *admissionv1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	// pods group is empty string
	if admissionSpec.Resource.Group != "" {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Resource != "pods" {
		contextLogger.Debug("Returning False, it's our group, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// TODO: add more needed rules
func shouldSetDefault(pod *corev1.Pod) bool {
	if _, ok := pod.Annotations[defaultKey]; ok {
		return false
	}

	if pod.Spec.HostNetwork == true {
		return false
	}

	return true
}
