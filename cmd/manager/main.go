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
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	machineactuator "github.com/openshift/cluster-api-provider-kubemark/pkg/actuators/machine"
	"github.com/openshift/cluster-api-provider-kubemark/pkg/apis/kubemarkproviderconfig/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	clusterapis "sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/controller/machine"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	watchNamespace := flag.String("namespace", "", "Namespace that the controller watches to reconcile machine-api objects. If unspecified, the controller watches for machine-api objects across all namespaces.")

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)
	flag.Parse()

	flag.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		glog.Fatal(err)
	}

	syncPeriod := 10 * time.Minute
	opts := manager.Options{
		SyncPeriod: &syncPeriod,
	}

	if *watchNamespace != "" {
		opts.Namespace = *watchNamespace
		klog.Infof("Watching machine-api objects only in namespace %q for reconciliation.", opts.Namespace)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, opts)
	if err != nil {
		glog.Fatal(err)
	}

	glog.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := clusterapis.AddToScheme(mgr.GetScheme()); err != nil {
		glog.Fatal(err)
	}

	machineActuator, err := initActuator(mgr)
	if err != nil {
		glog.Fatal(err)
	}

	if err := machine.AddWithActuator(mgr, machineActuator); err != nil {
		glog.Fatal(err)
	}

	glog.Info("Starting the Cmd.")

	// Start the Cmd
	glog.Fatal(mgr.Start(signals.SetupSignalHandler()))
}

func initActuator(mgr manager.Manager) (*machineactuator.Actuator, error) {
	codec, err := v1beta1.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("unable to create codec: %v", err)
	}

	params := machineactuator.ActuatorParams{
		Client:        mgr.GetClient(),
		Config:        mgr.GetConfig(),
		Codec:         codec,
		EventRecorder: mgr.GetEventRecorderFor("kubemark-controller"),
	}

	actuator, err := machineactuator.NewActuator(params)
	if err != nil {
		return nil, fmt.Errorf("could not create kubemark machine actuator: %v", err)
	}

	return actuator, nil
}
