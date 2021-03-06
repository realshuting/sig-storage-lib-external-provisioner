/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	"os"
	"path"

	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	provisionerName = "nirmata.io/hostpath"

	// following names are used to identify service via label
	// i.e. app=zk
	zk      = "zk"
	mongodb = "mongodb"
	es      = "es"
	kafka   = "kafka"

	defaultLeaderElection = false
)

type pvDirs struct {
	// *Dir is the directory to create PV-backing directories in
	zkDir      string
	esDir      string
	mongodbDir string
	kafkaDir   string
}

type hostPathProvisioner struct {
	pvDirs

	// Identity of this hostPathProvisioner, set to node's name. Used to identify
	// "this" provisioner's PVs.
	identity string
}

// NewHostPathProvisioner creates a new hostpath provisioner
func NewHostPathProvisioner() controller.Provisioner {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		klog.Fatal("env variable NODE_NAME must be set so that this provisioner can identify itself")
	}

	dirs := []string{"ZK_PV_DIR", "MONGODB_PV_DIR", "ES_PV_DIR", "KAFKA_PV_DIR"}
	dirsCache := make(map[string]string)
	for _, dir := range dirs {
		val := os.Getenv(dir)
		if val == "" && dir != "ES_PV_DIR" {
			klog.Fatalf("env variable %s must be set so that this provisioner knows where to place its data", dir)
		}
		dirsCache[dir] = val
	}

	return &hostPathProvisioner{
		pvDirs: pvDirs{
			zkDir:      dirsCache["ZK_PV_DIR"],
			mongodbDir: dirsCache["MONGODB_PV_DIR"],
			kafkaDir:   dirsCache["KAFKA_PV_DIR"],
		},
		identity: nodeName,
	}
}

var _ controller.Provisioner = &hostPathProvisioner{}

// Provision creates a storage asset and returns a PV object representing it.
func (p *hostPathProvisioner) Provision(options controller.ProvisionOptions) (*v1.PersistentVolume, error) {
	var pvDir string
	labels := options.PVC.GetLabels()

	switch labels["app"] {
	case zk:
		pvDir = p.zkDir
	case mongodb:
		pvDir = p.mongodbDir
	case es:
		pvDir = p.esDir
	case kafka:
		pvDir = p.kafkaDir
	default:
		pvDir = "/tmp/nirmata-hostpath-provisioner"
	}

	path := path.Join(pvDir, options.PVC.Namespace+"-"+options.PVC.Name+"-"+options.PVName)

	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
			Annotations: map[string]string{
				"hostPathProvisionerIdentity": p.identity,
				"hostpath":                    path,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: *options.StorageClass.ReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: path,
				},
			},
		},
	}

	klog.Infof("persistent volume %s is provisioned at %s\n", pv.GetName(), path)

	return pv, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *hostPathProvisioner) Delete(volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations["hostPathProvisionerIdentity"]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}

	path, ok := volume.Annotations["hostpath"]
	if !ok {
		return errors.New("hostpath annotation not found on PV")
	}

	if err := os.RemoveAll(path); err != nil {
		return err
	}

	return nil
}

func getLeaderElection() bool {
	if leaderEnv := os.Getenv("LEADER_ELECTION"); leaderEnv == "true" {
		return true
	}

	// leaderEnv set to false if env is empty or false
	return defaultLeaderElection
}
