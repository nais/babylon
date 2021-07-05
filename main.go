package main

import (
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func hello(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello world!")
}

func isAlive(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "OK")
}

func isReady(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "OK")
}

func Setup(level string) {
	log.SetFormatter(&log.JSONFormatter{FieldMap: log.FieldMap{
		log.FieldKeyMsg: "message",
	}})

	l, err := log.ParseLevel(level)
	if err != nil {
		log.Fatal(err)
	}

	log.SetLevel(l)
}

func main() {
	Setup("info")
	http.HandleFunc("/", hello)
	http.HandleFunc("/isReady", isReady)
	http.HandleFunc("/isAlive", isAlive)
	http.Handle("/metrics", promhttp.Handler())
	log.Info("Listening on http://localhost:8080")

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	_, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	log.Fatal(http.ListenAndServe(":8080", nil))
}
