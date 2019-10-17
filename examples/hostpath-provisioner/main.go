package main

import (
	"flag"
	"syscall"

	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

func main() {
	syscall.Umask(0)

	flag.Parse()
	flag.Set("logtostderr", "true")
	// Create an InClusterConfig and use it to create a client for the controller
	// to use to communicate with Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create client: %v", err)
	}

	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		klog.Fatalf("Error getting server version: %v", err)
	}

	// Create the provisioner: it implements the Provisioner interface expected by
	// the controller
	hostPathProvisioner := NewHostPathProvisioner()

	// Start the provision controller which will dynamically provision hostPath
	// PVs
	pc := controller.NewProvisionController(clientset, provisionerName, hostPathProvisioner,
		serverVersion.GitVersion, controller.LeaderElection(getLeaderElection()))
	pc.Run(wait.NeverStop)
}
