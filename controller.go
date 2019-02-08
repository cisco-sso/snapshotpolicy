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
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	snapshotv1alpha1 "github.com/cisco-sso/snapshotpolicy/pkg/apis/snapshotpolicy/v1alpha1"
	clientset "github.com/cisco-sso/snapshotpolicy/pkg/client/clientset/versioned"
	snapshotscheme "github.com/cisco-sso/snapshotpolicy/pkg/client/clientset/versioned/scheme"
	strategy "github.com/cisco-sso/snapshotpolicy/strategy"
	csiclientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
)

const (
	controllerAgentName = "snapshotpolicy-controller"
)

// Controller is the controller implementation for SnapshotPolicy resources
type controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeClientset kubernetes.Interface
	// snapshotPolicyClientset is a clientset for our own API group
	snapshotPolicyClientset clientset.Interface
	// Client for interacting with VolumeSnapshot resources that this controller schedules
	snapshotClient strategy.SnapshotClient
	// Indexer responsible for thread safe access to the obj store
	indexer cache.Indexer
	// Controller responsible for processing the FIFO queue of SnapshotPolicy objects
	// and calling provided hook functions
	controller cache.Controller
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
	// Map of `units` specified in snapshot policy to time.Duration representation
	durationMap map[string]time.Duration

	// Shared informer factory for informers and listers below
	kfac kubeinformers.SharedInformerFactory
	// Cache sync for PVC lister
	pvcHasSynced func() bool
	// PVC Lister
	pvcLister corelisters.PersistentVolumeClaimLister
}

// NewController returns a new sample controller
func NewController(
	kubeClientset kubernetes.Interface,
	snapshotPolicyClientset clientset.Interface,
	snapshotClient *rest.RESTClient,
	csiClientset csiclientset.Interface) *controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(snapshotscheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	resyncPeriod := time.Minute * 60
	kfac := kubeinformers.NewSharedInformerFactory(kubeClientset, resyncPeriod)

	controller := &controller{
		kubeClientset:           kubeClientset,
		snapshotPolicyClientset: snapshotPolicyClientset,
		recorder:                recorder,
		pvcHasSynced:            kfac.Core().V1().PersistentVolumeClaims().Informer().HasSynced,
		pvcLister:               kfac.Core().V1().PersistentVolumeClaims().Lister(),
	}
	controller.kfac = kfac

	// Watch snapshot objects
	source := cache.NewListWatchFromClient(
		snapshotPolicyClientset.SnapshotpolicyV1alpha1().RESTClient(),
		snapshotv1alpha1.SnapshotPolicyResourcePlural,
		corev1.NamespaceAll,
		fields.Everything())

	controller.snapshotClient = strategy.SnapshotClient{snapshotClient, csiClientset.VolumesnapshotV1alpha1(), useCSI}

	controller.indexer, controller.controller = cache.NewIndexerInformer(
		source,

		// The object type.
		&snapshotv1alpha1.SnapshotPolicy{},

		// resyncPeriod
		// if non-zero, will re-list this often (you will get OnUpdate
		//    calls, even if nothing changed). Otherwise, re-list will be delayed as
		//    long as possible (until the upstream source closes the watch or times out,
		//    or you stop the controller).
		resyncPeriod,

		// Watch Event Handlers
		cache.ResourceEventHandlerFuncs{
			AddFunc:    controller.addSnapshotPolicy,
			UpdateFunc: controller.updateSnapshotPolicy,
			DeleteFunc: controller.deleteSnapshotPolicy,
		},

		// Thread Safe indexer
		cache.Indexers{},
	)

	// Create a map of supported unit enumerations for quick duration lookup
	controller.durationMap = map[string]time.Duration{
		"minute": 1 * time.Minute,
		"hour":   1 * time.Hour,
		"day":    24 * time.Hour,
		"week":   7 * 24 * time.Hour,
	}

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *controller) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	glog.Info("Starting SnapshotPolicy controller")

	c.kfac.Start(stopCh)
	go c.controller.Run(stopCh)

	glog.Infof("Waiting for caches to sync for %s controller", "snapshot-policy")

	if !cache.WaitForCacheSync(stopCh, c.controller.HasSynced, c.pvcHasSynced) {
		utilruntime.HandleError(fmt.Errorf("Unable to sync caches for %s controller", "snapshot-policy"))
		return nil
	}

	c.setCacheDefaults()

	glog.Infof("Caches are synced for %s controller", "snapshot-policy")

	// Wait for scheduling volumesnapshot resources
	wait.Until(c.evaluatePolicies, time.Minute, stopCh)

	return nil
}

// setCacheDefaults - Set any missing struct values to reasonable defaults.
// Also calculate non-parsable `-` struct elements.
func (c *controller) setCacheDefaults() {
	for _, obj := range c.indexer.List() {
		if policy, ok := obj.(*snapshotv1alpha1.SnapshotPolicy); ok {
			c.setPolicyDefaults(policy)
		}
	}
}

func (c *controller) setPolicyDefaults(policy *snapshotv1alpha1.SnapshotPolicy) {

	if *policy.Spec.Period == 0 {
		*policy.Spec.Period = int32(1)
	}
	if policy.Spec.Unit == "" {
		policy.Spec.Unit = "day"
	}
	if *policy.Spec.Retention == 0 {
		*policy.Spec.Retention = int32(3)
	}
	if duration, ok := c.durationMap[strings.ToLower(policy.Spec.Unit)]; ok {
		policy.Spec.Duration = time.Duration(*policy.Spec.Period) * duration
	} else {
		policy.Spec.Duration = 24 * time.Hour
	}
	c.indexer.Update(policy)
}

func (c *controller) addSnapshotPolicy(obj interface{}) {
	if policy, ok := obj.(*snapshotv1alpha1.SnapshotPolicy); ok {
		glog.Infof("Policy %s/%s Added!",
			policy.GetObjectMeta().GetNamespace(),
			policy.GetObjectMeta().GetName())

		c.setPolicyDefaults(policy)
		//prevent creating snapshots on controller restarts for already initialized policies
		if _, err := time.Parse(time.RFC3339, policy.Status.LastSnapshotTime); err != nil {
			c.createSnapshot(*policy)
		}
	}
}

func (c *controller) updateSnapshotPolicy(oldObj, newObj interface{}) {
	if policy, ok := newObj.(*snapshotv1alpha1.SnapshotPolicy); ok {
		glog.Infof("Policy %s/%s Updated!",
			policy.GetObjectMeta().GetNamespace(),
			policy.GetObjectMeta().GetName())

		c.setPolicyDefaults(policy)
	}
}

func (c *controller) deleteSnapshotPolicy(obj interface{}) {
	if policy, ok := obj.(*snapshotv1alpha1.SnapshotPolicy); ok {
		glog.Infof("Policy %s/%s Deleted!",
			policy.GetObjectMeta().GetNamespace(),
			policy.GetObjectMeta().GetName())

		// TODO: This policy will automatically be removed from the cache.
		// Potentially worth considering a stop channel for certain long running strategies.
	}
}

func (c *controller) evaluatePolicies() {
	for _, obj := range c.indexer.List() {
		if policy, ok := obj.(*snapshotv1alpha1.SnapshotPolicy); ok {
			c.checkPolicyTime(*policy)
		}
	}
}

func (c *controller) updatePolicy(policy *snapshotv1alpha1.SnapshotPolicy) {
	_, err := c.snapshotPolicyClientset.
		Snapshotpolicy().
		SnapshotPolicies(policy.GetObjectMeta().GetNamespace()).
		Update(policy)
	if err != nil {
		glog.Errorf("Failed to update snapshot policy %s: %s", policy.GetObjectMeta().GetName(), err.Error())
	} else {
	}
}

func (c *controller) checkPolicyTime(policy snapshotv1alpha1.SnapshotPolicy) {
	lastUpdatedTime, err := time.Parse(time.RFC3339, policy.Status.LastSnapshotTime)
	if err != nil {
		glog.Infof("Defaulting Snapshot Creation Time.")
		lastUpdatedTime = policy.GetObjectMeta().GetCreationTimestamp().Time
	}
	glog.Infof("Checking policy %v", policy.Name)
	now := time.Now()

	// If the duration between snapshots has passed create a new one
	if now.After(lastUpdatedTime.Add(policy.Spec.Duration)) {
		c.createSnapshot(policy)
	}
}

func (c *controller) createSnapshot(policy snapshotv1alpha1.SnapshotPolicy) {
	glog.Infof("Creating snapshot for %s policy", policy.GetObjectMeta().GetName())
	strat, err := strategy.CreateStrategy(c.snapshotClient, c.pvcLister, strategy.StrategyName(policy.Spec.Strategy.Name))
	if err != nil {
		glog.Errorf("Error building strategy: %s", err.Error())
	}
	status, err := strat.Run(policy)
	if err != nil {
		c.recorder.Eventf(&policy, corev1.EventTypeWarning, strategy.EventRun, "%s failed strategy execution!", policy.GetObjectMeta().GetName())
		glog.Errorf("Error performing strategy: %s", err.Error())
		return
	}
	if status == nil {
		glog.Warningf("Didn't create any snapshots: %s", policy.Name)
	} else {
		c.recorder.Eventf(&policy, corev1.EventTypeNormal, strategy.EventRun, "%s successful strategy execution.", policy.GetObjectMeta().GetName())
		policy.Status = snapshotv1alpha1.SnapshotPolicyStatus{
			LastSnapshotTime: status.LastSnapshotTime,
		}
		c.updatePolicy(&policy)
	}
}
