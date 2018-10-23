/*
Copyright © 2018 Cisco Systems, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/cisco-sso/snapshotpolicy/pkg/client/clientset/versioned"
	"github.com/cisco-sso/snapshotpolicy/pkg/signals"
	volclient "github.com/kubernetes-incubator/external-storage/snapshot/pkg/client"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	masterURL  string
	kubeconfig string
)

func startMetricsServer(addr string) {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(addr, nil))
}

func main() {
	v := flag.String("logLevel", "INFO", "Logging Level")
	metrics := flag.String("metrics-address", ":8080", "Prometheus metrics hosting.")
	flag.Parse()
	flag.Set("stderrthreshold", *v)

	// Host metrics
	go startMetricsServer(*metrics)

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	snapshotClient, _, err := volclient.NewClient(cfg)
	if err != nil {
		glog.Fatalf("Could not get volumesnapshot client: %s", err.Error())
	}

	policyClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	controller := NewController(
		kubeClient,
		policyClient,
		snapshotClient)

	if err = controller.Run(stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
