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

package machine

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	providerconfigv1 "github.com/openshift/cluster-api-provider-kubemark/pkg/apis/kubemarkproviderconfig/v1beta1"
	v1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// providerConfigFromMachine gets the machine provider config MachineSetSpec from the
// specified cluster-api MachineSpec.
func providerConfigFromMachine(client client.Client, machine *v1alpha1.Machine, codec *providerconfigv1.KubemarkProviderConfigCodec) (*providerconfigv1.KubemarkMachineProviderConfig, error) {
	providerSpec := machine.Spec.ProviderSpec
	if providerSpec.Value == nil {
		return nil, fmt.Errorf("unable to find machine provider config: neither Spec.ProviderSpec.Value nor Spec.ProviderSpec.ValueFrom set")
	}

	var config providerconfigv1.KubemarkMachineProviderConfig
	if err := codec.DecodeProviderSpec(&v1alpha1.ProviderSpec{Value: providerSpec.Value}, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// isMaster returns true if the machine is part of a cluster's control plane
func isMaster(machine *v1alpha1.Machine) bool {
	if machineType, exists := machine.ObjectMeta.Labels[providerconfigv1.MachineTypeLabel]; exists && machineType == "master" {
		return true
	}
	return false
}

// updateConditionCheck tests whether a condition should be updated from the
// old condition to the new condition. Returns true if the condition should
// be updated.
type updateConditionCheck func(oldReason, oldMessage, newReason, newMessage string) bool

// updateConditionAlways returns true. The condition will always be updated.
func updateConditionAlways(_, _, _, _ string) bool {
	return true
}

// updateConditionNever return false. The condition will never be updated,
// unless there is a change in the status of the condition.
func updateConditionNever(_, _, _, _ string) bool {
	return false
}

// updateConditionIfReasonOrMessageChange returns true if there is a change
// in the reason or the message of the condition.
func updateConditionIfReasonOrMessageChange(oldReason, oldMessage, newReason, newMessage string) bool {
	return oldReason != newReason ||
		oldMessage != newMessage
}

func shouldUpdateCondition(
	oldStatus corev1.ConditionStatus, oldReason, oldMessage string,
	newStatus corev1.ConditionStatus, newReason, newMessage string,
	updateConditionCheck updateConditionCheck,
) bool {
	if oldStatus != newStatus {
		return true
	}
	return updateConditionCheck(oldReason, oldMessage, newReason, newMessage)
}

// setMachineProviderCondition sets the condition for the machine and
// returns the new slice of conditions.
// If the machine does not already have a condition with the specified type,
// a condition will be added to the slice if and only if the specified
// status is True.
// If the machine does already have a condition with the specified type,
// the condition will be updated if either of the following are true.
// 1) Requested status is different than existing status.
// 2) The updateConditionCheck function returns true.
func setMachineProviderCondition(
	conditions []providerconfigv1.KubemarkMachineProviderCondition,
	conditionType providerconfigv1.KubemarkMachineProviderConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck updateConditionCheck,
) []providerconfigv1.KubemarkMachineProviderCondition {
	now := metav1.Now()
	existingCondition := findKubemarkMachineProviderCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				providerconfigv1.KubemarkMachineProviderCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastProbeTime:      now,
				},
			)
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
		}
	}
	return conditions
}

// findKubemarkMachineProviderCondition finds in the machine the condition that has the
// specified condition type. If none exists, then returns nil.
func findKubemarkMachineProviderCondition(conditions []providerconfigv1.KubemarkMachineProviderCondition, conditionType providerconfigv1.KubemarkMachineProviderConditionType) *providerconfigv1.KubemarkMachineProviderCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
