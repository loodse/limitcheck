package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"

	"io/ioutil"

	"encoding/json"

	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	certFile      = flag.String("tlsCertFile", "", "")
	keyFile       = flag.String("tlsKeyFile", "", "")
	port          = flag.Int("port", 443, "")
)

func getAdmitResponse(ar v1beta1.AdmissionReview) *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		Response: &v1beta1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: true,
			Result: &metav1.Status{
				Message: "OK",
			},
		},
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	log.Println("request handling...")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	ar := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		payload, err := json.Marshal(&v1beta1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		})
		if err != nil {
			log.Printf("Can not marshal response: %q", err)
		}
		w.Write(payload)
		return
	}

	admitResponse := getAdmitResponse(ar)

	log.Printf("Got request for %q", ar.Request.Kind.Kind)
	if ar.Request.Kind.Kind == "Pod" {
		pod := v1.Pod{}
		err = json.Unmarshal(ar.Request.Object.Raw, &pod)
		if err != nil {
			log.Printf("Did not get a `pod`: %q", err.Error())
			return
		}
		podName := pod.Name
		for _, container := range pod.Spec.Containers {
			containerName := container.Name
			cpuLimit, memLimit := (*container.Resources.Limits.Cpu()).Value(), (*container.Resources.Limits.Memory()).Value()

			if cpuLimit == 0 || memLimit == 0 {
				admitResponse.Response.Allowed = false
				admitResponse.Response.Result = &metav1.Status{
					Message: fmt.Sprintf("%s.%s doesn't have ressource limits", podName, containerName),
				}
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	payload, err := json.Marshal(admitResponse)
	if err != nil {
		log.Printf("Can not marshal response: %q", err)
	}
	w.Write(payload)
	return
}

func main() {
	flag.Parse()
	pair, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("Error loading certs: %q", err.Error())
	}
	log.Println("Successful loaded TLS key pair")
	server := &http.Server{
		Addr:      fmt.Sprintf(":%v", *port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/validate", handler)
	server.Handler = mux
	log.Println("Handler registered")
	log.Printf("Serving on port :%d", *port)
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatalf("Could not start server: %q", err.Error())
	}
}
