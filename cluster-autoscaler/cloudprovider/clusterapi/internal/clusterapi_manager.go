//go:generate ./vendor-cluster-api.sh

package internal

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/clusterapi/types"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	v1alpha1apis "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

const (
	refreshInterval = 30 * time.Second
)

type clusterManager struct {
	lastRefresh          time.Time
	clusterSnapshot      *clusterSnapshot
	clusterSnapshotMutex sync.Mutex
	clusterapi           v1alpha1apis.ClusterV1alpha1Interface
	kubeclient           *kubeclient.Clientset
	resourceLimits       *cloudprovider.ResourceLimiter
}

func init() {
	spew.Config = spew.ConfigState{
		DisablePointerAddresses: true,
		DisableCapacities:       true,
		SortKeys:                true,
		SpewKeys:                true,
		Indent:                  "  ",
	}
}

func (m *clusterManager) Cleanup() error {
	return nil
}

func (m *clusterManager) GetResourceLimiter() (*cloudprovider.ResourceLimiter, error) {
	return m.resourceLimits, nil
}

func (m *clusterManager) GetMachineSets(namespace string) ([]types.MachineSet, error) {
	result := []types.MachineSet{}

	for _, ms := range m.getClusterState().MachineSetMap {
		if ms.MaxSize()-ms.MinSize() > 0 {
			result = append(result, ms)
		}
	}

	glog.Infof("NODE GROUPS %v", spew.Sdump(result))

	return result, nil
}

func (m *clusterManager) MachineSetForNode(nodename string) (types.MachineSet, error) {
	snapshot := m.getClusterState()
	if key, exists := snapshot.NodeToMachineSetMap[nodename]; exists {
		glog.Infof("MachineSetForNode[%q]=%q", nodename, key)
		return snapshot.MachineSetMap[key], nil
	}
	return nil, nil
}

func (m *clusterManager) getClusterState() *clusterSnapshot {
	m.clusterSnapshotMutex.Lock()
	defer m.clusterSnapshotMutex.Unlock()
	return m.clusterSnapshot
}

func (m *clusterManager) setClusterState(s *clusterSnapshot) {
	m.clusterSnapshotMutex.Lock()
	defer m.clusterSnapshotMutex.Unlock()
	m.clusterSnapshot = s
}

func (m *clusterManager) Refresh() error {
	if m.lastRefresh.Add(refreshInterval).After(time.Now()) && m.clusterSnapshot != nil {
		return nil
	}

	s, err := getClusterSnaphot(m)
	if err == nil {
		m.lastRefresh = time.Now()
		glog.Infof("cluster refreshed at %v\n%v", m.lastRefresh, spew.Sdump(s))
		m.setClusterState(s)
	}

	return err
}

func NewClusterManager(do cloudprovider.NodeGroupDiscoveryOptions) (*clusterManager, error) {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		kubeconfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	kubeclient, err := kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	clusterapi, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("could not create client for talking to the apiserver: %v", err)
	}

	return &clusterManager{
		clusterSnapshot: newEmptySnapshot(),
		clusterapi:      clusterapi.ClusterV1alpha1(),
		kubeclient:      kubeclient,
	}, nil
}
