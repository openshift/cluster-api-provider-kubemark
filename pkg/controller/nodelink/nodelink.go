package nodelink

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"k8s.io/klog/glog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	v1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	nodecontroller "sigs.k8s.io/cluster-api/pkg/controller/node"
)

const (
	machineInternalIPIndex = "machineInternalIPIndex"
)

// Add creates a new Node Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	if err := mgr.GetCache().IndexField(&v1alpha1.Machine{}, machineInternalIPIndex, indexMachineByInternalIP); err != nil {
		return nil, fmt.Errorf("unable to add indexer to machine informer: %v", err)
	}

	return &ReconcileNode{
		Client: mgr.GetClient(),
	}, nil
}

func indexMachineByInternalIP(obj runtime.Object) []string {
	if machine, ok := obj.(*v1alpha1.Machine); ok {
		addresses := []string{}
		for _, a := range machine.Status.Addresses {
			if a.Type == corev1.NodeInternalIP {
				addresses = append(addresses, a.Address)
			}
		}
		return addresses
	}
	return []string{}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodelink-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Node
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNode{}

// ReconcileNode reconciles a Node object
type ReconcileNode struct {
	client.Client
}

func (r *ReconcileNode) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Node node
	node := &corev1.Node{}
	err := r.Get(context.Background(), request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			klog.Errorf("Unable to retrieve Node %v from store: %v", request, err)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.Infof("Processing node %q", node.Name)

	if !node.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	// Find this nodes internal IP so we can search for a matching machine
	machines := []*v1alpha1.Machine{}
	for _, a := range node.Status.Addresses {
		if a.Type == corev1.NodeInternalIP {
			machine, err := r.machineByInternalIP(a.Address)
			if err != nil {
				glog.Warningf("Unable to find a machine for node %q: %v", node.Name, err)
				return reconcile.Result{}, err
			}
			if machine == nil {
				continue
			}
			exists := false
			for _, m := range machines {
				if (types.NamespacedName{Namespace: m.Namespace, Name: m.Name}).String() == (types.NamespacedName{Namespace: machine.Namespace, Name: machine.Name}).String() {
					exists = true
				}
			}
			if !exists {
				machines = append(machines, machine)
			}
		}
	}

	switch n := len(machines); {
	case n == 0:
		glog.Errorf("Unable to find a machine for node %q", node.Name)
		return reconcile.Result{}, fmt.Errorf("unable to find a machine for node: %q", node.Name)
	case n > 1:
		glog.Errorf("Find more than 1 machine for node %q", node.Name)
		return reconcile.Result{}, fmt.Errorf("found more than 1 machine for node: %q", node.Name)
	}
	matchingMachine := machines[0]

	machinePairKey := (types.NamespacedName{Namespace: matchingMachine.Namespace, Name: matchingMachine.Name}).String()
	klog.V(3).Infof("Found machine %s for node %s", machinePairKey, node.Name)
	modNode := node.DeepCopy()
	if modNode.Annotations == nil {
		modNode.Annotations = map[string]string{}
	}
	modNode.Annotations[nodecontroller.MachineAnnotationKey] = machinePairKey

	if modNode.Labels == nil {
		modNode.Labels = map[string]string{}
	}

	for k, v := range matchingMachine.Spec.Labels {
		klog.V(3).Infof("Copying label %s = %s", k, v)
		modNode.Labels[k] = v
	}

	addTaintsToNode(modNode, matchingMachine)

	if !reflect.DeepEqual(node, modNode) {
		klog.V(3).Infof("Node %q has changed, patching", modNode.Name)
		if err := r.Patch(context.Background(), modNode, client.MergeFrom(node)); err != nil {
			klog.Errorf("Error updating node: %v", err)
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// addTaintsToNode adds taints from machine object to the node object
// Taints are to be an authoritative list on the machine spec per cluster-api comments.
// However, we believe many components can directly taint a node and there is no direct source of truth that should enforce a single writer of taints
func addTaintsToNode(node *corev1.Node, machine *v1alpha1.Machine) {
	for _, mTaint := range machine.Spec.Taints {
		glog.V(3).Infof("Adding taint %v from machine %q to node %q", mTaint, machine.Name, node.Name)
		alreadyPresent := false
		for _, nTaint := range node.Spec.Taints {
			if nTaint.Key == mTaint.Key && nTaint.Effect == mTaint.Effect {
				glog.V(3).Infof("Skipping to add machine taint, %v, to the node. Node already has a taint with same key and effect", mTaint)
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			node.Spec.Taints = append(node.Spec.Taints, mTaint)
		}
	}
}

func (r *ReconcileNode) machineByInternalIP(nodeInternalIP string) (*v1alpha1.Machine, error) {
	klog.V(4).Infof("Searching machine cache for IP %q match", nodeInternalIP)

	machineList := &v1alpha1.MachineList{}
	if err := r.List(context.TODO(), machineList, client.MatchingField(machineInternalIPIndex, nodeInternalIP)); err != nil {
		return nil, err
	}

	switch n := len(machineList.Items); {
	case n == 0:
		return nil, nil
	case n > 1:
		return nil, fmt.Errorf("internal error; expected 1 machine, got %v", n)
	}

	return &machineList.Items[0], nil
}
