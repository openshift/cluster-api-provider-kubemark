package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"os/exec"

	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	machineactuator "github.com/openshift/cluster-api-provider-kubemark/pkg/actuators/machine"
	"github.com/openshift/cluster-api-provider-kubemark/pkg/apis/kubemarkproviderconfig/v1beta1"
	v1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type manifestParams struct {
	ClusterID string
}

func readMachineManifest(manifestParams *manifestParams, manifestLoc string) (*v1alpha1.Machine, error) {
	machine := v1alpha1.Machine{}
	manifestBytes, err := ioutil.ReadFile(manifestLoc)
	if err != nil {
		return nil, fmt.Errorf("unable to read %v: %v", manifestLoc, err)
	}

	t, err := template.New("machineuserdata").Parse(string(manifestBytes))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = t.Execute(&buf, *manifestParams)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(buf.Bytes(), &machine); err != nil {
		return nil, fmt.Errorf("unable to unmarshal %v: %v", manifestLoc, err)
	}

	return &machine, nil
}

func readClusterResources(manifestParams *manifestParams, clusterLoc, machineLoc string) (*v1alpha1.Cluster, *v1alpha1.Machine, error) {
	machine, err := readMachineManifest(manifestParams, machineLoc)
	if err != nil {
		return nil, nil, err
	}

	cluster := &v1alpha1.Cluster{}
	bytes, err := ioutil.ReadFile(clusterLoc)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster manifest %q: %v", clusterLoc, err)
	}

	if err := yaml.Unmarshal(bytes, &cluster); err != nil {
		return nil, nil, fmt.Errorf("cluster manifest %q: %v", clusterLoc, err)
	}

	return cluster, machine, nil
}

// CreateActuator creates actuator with fake clientsets
func createActuator(machine *v1alpha1.Machine, kubeconfig string) (*machineactuator.Actuator, error) {
	objList := []runtime.Object{machine}
	fakeClient := fake.NewFakeClient(objList...)

	codec, err := v1beta1.NewCodec()
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return nil, err
	}

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}

	params := machineactuator.ActuatorParams{
		Client: fakeClient,
		Config: restConfig,
		Codec:  codec,
		// use empty recorder dropping any event recorded
		EventRecorder: &record.FakeRecorder{},
	}

	actuator, err := machineactuator.NewActuator(params)
	if err != nil {
		return nil, err
	}
	return actuator, nil
}

func cmdRun(binaryPath string, args ...string) ([]byte, error) {
	cmd := exec.Command(binaryPath, args...)
	return cmd.CombinedOutput()
}
