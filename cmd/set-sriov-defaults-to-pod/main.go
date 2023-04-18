package main

import (
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	webhooks "github.com/openshift/set-sriov-defaults-to-pod/pkg/webhooks"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func main() {
	log.Info("Starting Webhooks.")

	log.SetLevel(log.InfoLevel)

	decoder := createDecoder()

	admissionCmd.RunAdmissionServer(
		////mutating webhooks
		webhooks.NewPodSRIOVMutatingAdmissionHook(decoder),
	)
}

func createDecoder() *admission.Decoder {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		log.WithError(err).Fatal("could not add to corev1 scheme")
	}
	decoder, err := admission.NewDecoder(scheme)
	if err != nil {
		log.WithError(err).Fatal("could not create a decoder")
	}
	return decoder
}
