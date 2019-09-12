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

package u

import (
	"fmt"

	clusterclient "github.com/openshift/cluster-api/pkg/client/clientset_generated/clientset"
	clusterinformers "github.com/openshift/cluster-api/pkg/client/informers_generated/externalversions"
	machinev1beta1 "github.com/openshift/cluster-api/pkg/client/informers_generated/externalversions/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
)

const (
	machineProviderIDIndex = "openshiftmachineapi-machineProviderIDIndex"
	nodeProviderIDIndex    = "openshiftmachineapi-nodeProviderIDIndex"
)

// machineController watches for Nodes, Machines, MachineSets and
// MachineDeployments as they are added, updated and deleted on the
// cluster. Additionally, it adds indices to the node informers to
// satisfy lookup by node.Spec.ProviderID.
type machineController struct {
	clusterClientset          clusterclient.Interface
	clusterInformerFactory    clusterinformers.SharedInformerFactory
	kubeInformerFactory       kubeinformers.SharedInformerFactory
	machineDeploymentInformer machinev1beta1.MachineDeploymentInformer
	machineInformer           machinev1beta1.MachineInformer
	machineSetInformer        machinev1beta1.MachineSetInformer
	nodeInformer              cache.SharedIndexInformer
	enableMachineDeployments  bool
}

type machineSetFilterFunc func(machineSet *MachineSet) error

func indexMachineByProviderID(obj interface{}) ([]string, error) {
	if machine, ok := obj.(*Machine); ok {
		if machine.Spec.ProviderID != nil && *machine.Spec.ProviderID != "" {
			return []string{*machine.Spec.ProviderID}, nil
		}
		return []string{}, nil
	}
	return []string{}, nil
}

func indexNodeByProviderID(obj interface{}) ([]string, error) {
	if node, ok := obj.(*corev1.Node); ok {
		if node.Spec.ProviderID != "" {
			return []string{node.Spec.ProviderID}, nil
		}
		return []string{}, nil
	}
	return []string{}, nil
}

func (c *machineController) findMachine(id string) (*Machine, error) {
	item, exists, err := c.machineInformer.Informer().GetStore().GetByKey(id)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	machine, ok := item.(*Machine)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", machine)
	}

	return machine.DeepCopy(), nil
}

func (c *machineController) findMachineDeployment(id string) (*MachineDeployment, error) {
	item, exists, err := c.machineDeploymentInformer.Informer().GetStore().GetByKey(id)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	machineDeployment, ok := item.(*MachineDeployment)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", machineDeployment)
	}

	return machineDeployment.DeepCopy(), nil
}

// findMachineOwner returns the machine set owner for machine, or nil
// if there is no owner. A DeepCopy() of the object is returned on
// success.
func (c *machineController) findMachineOwner(machine *Machine) (*MachineSet, error) {
	machineOwnerRef := machineOwnerRef(machine)
	if machineOwnerRef == nil {
		return nil, nil
	}

	store := c.machineSetInformer.Informer().GetStore()
	item, exists, err := store.GetByKey(fmt.Sprintf("%s/%s", machine.Namespace, machineOwnerRef.Name))
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	machineSet, ok := item.(*MachineSet)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type: %T", machineSet)
	}

	if !machineIsOwnedByMachineSet(machine, machineSet) {
		return nil, nil
	}

	return machineSet.DeepCopy(), nil
}

// run starts shared informers and waits for the informer cache to
// synchronize.
func (c *machineController) run(stopCh <-chan struct{}) error {
	c.kubeInformerFactory.Start(stopCh)
	c.clusterInformerFactory.Start(stopCh)

	syncFuncs := []cache.InformerSynced{
		c.nodeInformer.HasSynced,
		c.machineInformer.Informer().HasSynced,
		c.machineSetInformer.Informer().HasSynced,
	}

	if c.enableMachineDeployments {
		syncFuncs = append(syncFuncs, c.machineDeploymentInformer.Informer().HasSynced)
	}

	klog.V(4).Infof("waiting for caches to sync")
	if !cache.WaitForCacheSync(stopCh, syncFuncs...) {
		return fmt.Errorf("syncing caches failed")
	}

	return nil
}

// findMachineByProviderID finds machine matching providerID. A
// DeepCopy() of the object is returned on success.
func (c *machineController) findMachineByProviderID(providerID string) (*Machine, error) {
	objs, err := c.machineInformer.Informer().GetIndexer().ByIndex(machineProviderIDIndex, providerID)
	if err != nil {
		return nil, err
	}

	switch n := len(objs); {
	case n > 1:
		return nil, fmt.Errorf("internal error; expected len==1, got %v", n)
	case n == 1:
		machine, ok := objs[0].(*Machine)
		if !ok {
			return nil, fmt.Errorf("internal error; unexpected type %T", machine)
		}
		if machine != nil {
			return machine.DeepCopy(), nil
		}
	}

	// If the machine object has no providerID--maybe actuator
	// does not set this value (e.g., OpenStack)--then first
	// lookup the node using ProviderID. If that is successful
	// then the machine can be found using the annotation (should
	// it exist).
	node, err := c.findNodeByProviderID(providerID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	return c.findMachine(node.Annotations[machineAnnotationKey])
}

// findNodeByNodeName finds the Node object keyed by name.. Returns
// nil if it cannot be found. A DeepCopy() of the object is returned
// on success.
func (c *machineController) findNodeByNodeName(name string) (*corev1.Node, error) {
	item, exists, err := c.nodeInformer.GetIndexer().GetByKey(name)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	node, ok := item.(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", node)
	}

	return node.DeepCopy(), nil
}

// machinesInMachineSet returns all the machines that belong to
// machineSet. For each machine in the set a DeepCopy() of the object
// is returned.
func (c *machineController) machinesInMachineSet(machineSet *MachineSet) ([]*Machine, error) {
	listOptions := labels.SelectorFromSet(labels.Set(machineSet.Labels))
	machines, err := c.listMachines(listOptions)
	if err != nil {
		return nil, err
	}

	var result []*Machine

	for _, machine := range machines {
		if machineIsOwnedByMachineSet(machine, machineSet) {
			result = append(result, machine.DeepCopy())
		}
	}

	return result, nil
}

// newMachineController constructs a controller that watches Nodes,
// Machines and MachineSet as they are added, updated and deleted on
// the cluster.
func newMachineController(
	kubeclient kubeclient.Interface,
	clusterclient clusterclient.Interface,
	enableMachineDeployments bool,
) (*machineController, error) {
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, 0)
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterclient, 0)

	// var machineDeploymentInformer machineMachineDeploymentInformer
	// if enableMachineDeployments {
	// 	machineDeploymentInformer = clusterInformerFactory.Machine().V1beta1().MachineDeployments()
	// 	machineDeploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	// }

	// machineInformer := clusterInformerFactory.Machine().V1beta1().Machines()
	// machineSetInformer := clusterInformerFactory.Machine().V1beta1().MachineSets()

	// machineInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})
	// machineSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	nodeInformer := kubeInformerFactory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{})

	// if err := machineInformer.Informer().GetIndexer().AddIndexers(cache.Indexers{
	// 	machineProviderIDIndex: indexMachineByProviderID,
	// }); err != nil {
	// 	return nil, fmt.Errorf("cannot add machine indexer: %v", err)
	// }

	if err := nodeInformer.GetIndexer().AddIndexers(cache.Indexers{
		nodeProviderIDIndex: indexNodeByProviderID,
	}); err != nil {
		return nil, fmt.Errorf("cannot add node indexer: %v", err)
	}

	return &machineController{
		clusterClientset:       clusterclient,
		clusterInformerFactory: clusterInformerFactory,
		kubeInformerFactory:    kubeInformerFactory,
		// machineDeploymentInformer: machineDeploymentInformer,
		// machineInformer:           machineInformer,
		// machineSetInformer:        machineSetInformer,
		nodeInformer:             nodeInformer,
		enableMachineDeployments: enableMachineDeployments,
	}, nil
}

func (c *machineController) machineSetNodeNames(machineSet *MachineSet) ([]string, error) {
	machines, err := c.machinesInMachineSet(machineSet)
	if err != nil {
		return nil, fmt.Errorf("error listing machines: %v", err)
	}

	var nodes []string

	for _, machine := range machines {
		if machine.Spec.ProviderID != nil && *machine.Spec.ProviderID != "" {
			// Prefer machine<=>node mapping using ProviderID
			node, err := c.findNodeByProviderID(*machine.Spec.ProviderID)
			if err != nil {
				return nil, err
			}
			if node != nil {
				nodes = append(nodes, node.Spec.ProviderID)
				continue
			}
		}

		if machine.Status.NodeRef == nil {
			klog.V(4).Infof("Status.NodeRef of machine %q is currently nil", machine.Name)
			continue
		}
		if machine.Status.NodeRef.Kind != "Node" {
			klog.Errorf("Status.NodeRef of machine %q does not reference a node (rather %q)", machine.Name, machine.Status.NodeRef.Kind)
			continue
		}

		node, err := c.findNodeByNodeName(machine.Status.NodeRef.Name)
		if err != nil {
			return nil, fmt.Errorf("unknown node %q", machine.Status.NodeRef.Name)
		}

		if node != nil {
			nodes = append(nodes, node.Spec.ProviderID)
		}
	}

	klog.V(4).Infof("nodegroup %s has nodes %v", machineSet.Name, nodes)

	return nodes, nil
}

func (c *machineController) filterAllMachineSets(f machineSetFilterFunc) error {
	return c.filterMachineSets(metav1.NamespaceAll, f)
}

func (c *machineController) filterMachineSets(namespace string, f machineSetFilterFunc) error {
	machineSets, err := c.listMachineSets()
	if err != nil {
		return nil
	}
	for _, machineSet := range machineSets {
		if err := f(machineSet); err != nil {
			return err
		}
	}
	return nil
}

func (c *machineController) machineSetNodeGroups() ([]*nodegroup, error) {
	var nodegroups []*nodegroup

	if err := c.filterAllMachineSets(func(machineSet *MachineSet) error {
		if machineSetHasMachineDeploymentOwnerRef(machineSet) {
			return nil
		}
		ng, err := newNodegroupFromMachineSet(c, machineSet.DeepCopy())
		if err != nil {
			return err
		}
		if ng.MaxSize()-ng.MinSize() > 0 && pointer.Int32PtrDerefOr(machineSet.Spec.Replicas, 0) > 0 {
			nodegroups = append(nodegroups, ng)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return nodegroups, nil
}

func (c *machineController) machineDeploymentNodeGroups() ([]*nodegroup, error) {
	if !c.enableMachineDeployments {
		return nil, nil
	}

	machineDeployments, err := c.listMachineDeployments()
	if err != nil {
		return nil, err
	}

	var nodegroups []*nodegroup

	for _, md := range machineDeployments {
		ng, err := newNodegroupFromMachineDeployment(c, md.DeepCopy())
		if err != nil {
			return nil, err
		}
		// add nodegroup iff it has the capacity to scale
		if ng.MaxSize()-ng.MinSize() > 0 && pointer.Int32PtrDerefOr(md.Spec.Replicas, 0) > 0 {
			nodegroups = append(nodegroups, ng)
		}
	}

	return nodegroups, nil
}

func (c *machineController) nodeGroups() ([]*nodegroup, error) {
	machineSets, err := c.machineSetNodeGroups()
	if err != nil {
		return nil, err
	}

	machineDeployments, err := c.machineDeploymentNodeGroups()
	if err != nil {
		return nil, err
	}
	return append(machineSets, machineDeployments...), nil
}

func (c *machineController) nodeGroupForNode(node *corev1.Node) (*nodegroup, error) {
	machine, err := c.findMachineByProviderID(node.Spec.ProviderID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, nil
	}

	machineSet, err := c.findMachineOwner(machine)
	if err != nil {
		return nil, err
	}

	if machineSet == nil {
		return nil, nil
	}

	if c.enableMachineDeployments {
		if ref := machineSetMachineDeploymentRef(machineSet); ref != nil {
			key := fmt.Sprintf("%s/%s", machineSet.Namespace, ref.Name)
			machineDeployment, err := c.findMachineDeployment(key)
			if err != nil {
				return nil, fmt.Errorf("unknown MachineDeployment %q: %v", key, err)
			}
			if machineDeployment == nil {
				return nil, fmt.Errorf("unknown MachineDeployment %q", key)
			}
			nodegroup, err := newNodegroupFromMachineDeployment(c, machineDeployment)
			if err != nil {
				return nil, fmt.Errorf("failed to build nodegroup for node %q: %v", node.Name, err)
			}
			// We don't scale from 0 so nodes must belong
			// to a nodegroup that has a scale size of at
			// least 1.
			if nodegroup.MaxSize()-nodegroup.MinSize() < 1 {
				return nil, nil
			}
			return nodegroup, nil
		}
	}

	nodegroup, err := newNodegroupFromMachineSet(c, machineSet)
	if err != nil {
		return nil, fmt.Errorf("failed to build nodegroup for node %q: %v", node.Name, err)
	}

	// We don't scale from 0 so nodes must belong to a nodegroup
	// that has a scale size of at least 1.
	if nodegroup.MaxSize()-nodegroup.MinSize() < 1 {
		return nil, nil
	}

	klog.V(4).Infof("node %q is in nodegroup %q", node.Name, machineSet.Name)
	return nodegroup, nil
}

// findNodeByProviderID find the Node object keyed by provideID.
// Returns nil if it cannot be found. A DeepCopy() of the object is
// returned on success.
func (c *machineController) findNodeByProviderID(providerID string) (*corev1.Node, error) {
	objs, err := c.nodeInformer.GetIndexer().ByIndex(nodeProviderIDIndex, providerID)
	if err != nil {
		return nil, err
	}

	switch n := len(objs); {
	case n == 0:
		return nil, nil
	case n > 1:
		return nil, fmt.Errorf("internal error; expected len==1, got %v", n)
	}

	node, ok := objs[0].(*corev1.Node)
	if !ok {
		return nil, fmt.Errorf("internal error; unexpected type %T", node)
	}

	return node.DeepCopy(), nil
}

func (c *machineController) listMachines(options labels.Selector) ([]*Machine, error) {
	return nil, nil
}

func (c *machineController) listMachineSets() ([]*MachineSet, error) {
	return nil, nil
}

func (c *machineController) listMachineDeployments() ([]*MachineDeployment, error) {
	return nil, nil
}

func (c *machineController) updateMachine(machine *Machine) (*Machine, error) {
	return machine, nil
}
