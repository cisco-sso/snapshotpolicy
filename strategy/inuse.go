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

package strategy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"

	snapshotpolicy "github.com/cisco-sso/snapshotpolicy/pkg/apis/snapshotpolicy"
	v1alpha1 "github.com/cisco-sso/snapshotpolicy/pkg/apis/snapshotpolicy/v1alpha1"
	uuid "github.com/google/uuid"
	crdv1 "github.com/kubernetes-incubator/external-storage/snapshot/pkg/apis/crd/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
)

// The in use strategy simply creates a volume snapshot with the annotation that will set the force option
type inUseStragey struct {
	client    *rest.RESTClient
	pvcLister corelisters.PersistentVolumeClaimLister
}

const (
	PolicyLabelKey = snapshotpolicy.GroupName + "/inuse"
	ClaimLabelKey  = snapshotpolicy.GroupName + "/claim"
	ForceKey       = "snapshot.alpha.kubernetes.io/force"
)

func (strat *inUseStragey) createSnapForPVC(policy v1alpha1.SnapshotPolicy, pvcName string) (*v1alpha1.SnapshotPolicyStatus, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		glog.Errorln(err.Error())
		return nil, err
	}

	var result crdv1.VolumeSnapshot
	err = strat.client.Post().
		Resource(crdv1.VolumeSnapshotResourcePlural).
		Namespace(policy.GetObjectMeta().GetNamespace()).
		Body(&crdv1.VolumeSnapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: crdv1.GroupName + "/v1",
				Kind:       "VolumeSnapshot",
			},
			Metadata: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s", pvcName, uuid.String()),
				Labels: map[string]string{
					PolicyLabelKey: policy.GetObjectMeta().GetName(),
					ClaimLabelKey:  pvcName,
				},
				Annotations: map[string]string{
					ForceKey: "true",
				},
			},
			Spec: crdv1.VolumeSnapshotSpec{
				PersistentVolumeClaimName: pvcName,
			},
		}).
		Do().Into(&result)
	if err != nil {
		return nil, err
	} else {
		glog.Infof("Created new volumesnapshot %s for policy %s.",
			result.GetObjectMeta().GetName(),
			policy.GetObjectMeta().GetName())
	}
	var status v1alpha1.SnapshotPolicyStatus
	status.LastSnapshotTime = result.GetObjectMeta().GetCreationTimestamp().Format(time.RFC3339)
	return &status, nil
}

func (strat *inUseStragey) Run(policy v1alpha1.SnapshotPolicy) (*v1alpha1.SnapshotPolicyStatus, error) {
	pvcNames, retErr := strat.getPvcNames(policy)
	if retErr != nil {
		glog.Errorf("Unable to get PVCs: %v", retErr)
	}
	defer strat.expireOld(policy, pvcNames)
	var retStatus *v1alpha1.SnapshotPolicyStatus
	for _, pvcName := range pvcNames {
		status, err := strat.createSnapForPVC(policy, pvcName)
		// Compile any error messages and still attempt to continue
		if err != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%s;%s", retErr.Error(), err.Error())
			} else {
				retErr = err
			}
		} else {
			// Only capture the first success (oldest one)
			if retStatus == nil {
				retStatus = new(v1alpha1.SnapshotPolicyStatus)
				retStatus.LastSnapshotTime = status.LastSnapshotTime
			}
		}
	}

	return retStatus, retErr
}

func (strat *inUseStragey) getPvcNames(policy v1alpha1.SnapshotPolicy) ([]string, error) {
	pvcNames := make(map[string]bool)
	if policy.Spec.PVCNames != nil {
		for _, pvcName := range *policy.Spec.PVCNames {
			pvcNames[pvcName] = true
		}
	}
	var errors []string
	if policy.Spec.PVCLabelSelectors != nil {
		for _, s := range *policy.Spec.PVCLabelSelectors {
			selector, err := metav1.LabelSelectorAsSelector(&s)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Selector %v: %v", s, err))
				continue
			}
			pvcs, err := strat.pvcLister.PersistentVolumeClaims(policy.Namespace).List(selector)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Selector %v: %v", selector, err))
				continue
			}
			for _, pvc := range pvcs {
				pvcNames[pvc.Name] = true
			}
		}
	}

	pvcArray := make([]string, 0, len(pvcNames))
	for pvcName, _ := range pvcNames {
		pvcArray = append(pvcArray, pvcName)
	}
	var err error
	if len(errors) > 0 {
		err = fmt.Errorf("Failed to get PVCs for policy %v/%v [%v]", policy.Namespace, policy.Name, strings.Join(errors, ", "))
	}
	return pvcArray, err
}

type snapshotDate struct {
	Snapshot crdv1.VolumeSnapshot
	Time     metav1.Time
}

// dateList - A reverse sortable array of snapshotDate
type dateList []snapshotDate

func (s *dateList) Len() int {
	return len(*s)
}

func (s *dateList) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

func (s *dateList) Less(i, j int) bool {
	return (*s)[i].Time.Time.After((*s)[j].Time.Time)
}

func (strat *inUseStragey) GetSortedSnapshots(policy v1alpha1.SnapshotPolicy, claimName string) []snapshotDate {
	existingSnaps := crdv1.VolumeSnapshotList{}
	err := strat.client.
		Get().
		Resource(crdv1.VolumeSnapshotResourcePlural).
		Do().Into(&existingSnaps)
	dates := make(dateList, 0)
	if err != nil {
		glog.Infoln(err.Error())
		return dates
	}
	for _, volSnap := range existingSnaps.Items {
		// Only get the snapshots for my policy, for this claim
		// TODO: Label selectors would be better, however strat.client does not have a clientset
		if policyLabel, ok := volSnap.GetObjectMeta().GetLabels()[PolicyLabelKey]; ok {
			if claimLabel, ok := volSnap.GetObjectMeta().GetLabels()[ClaimLabelKey]; ok {
				if policyLabel == policy.GetObjectMeta().GetName() && claimLabel == claimName {
					dates = append(dates,
						snapshotDate{
							Snapshot: volSnap,
							Time:     volSnap.GetObjectMeta().GetCreationTimestamp(),
						})
				}
			}
		}
	}
	sort.Sort(&dates)
	return dates
}

func (strat *inUseStragey) deleteSnapshot(snapshot crdv1.VolumeSnapshot) {
	result := strat.client.
		Delete().
		Name(snapshot.GetObjectMeta().GetName()).
		Namespace(snapshot.GetObjectMeta().GetNamespace()).
		Resource(crdv1.VolumeSnapshotResourcePlural).
		Do()
	if result.Error() != nil {
		glog.Errorf("Failed to delete snapshot %s: %s", snapshot.GetObjectMeta().GetName(), result.Error().Error())
	}
}

func (strat *inUseStragey) expireOld(policy v1alpha1.SnapshotPolicy, pvcNames []string) {
	for _, claim := range pvcNames {
		datedSnapshots := strat.GetSortedSnapshots(policy, claim)
		if len(datedSnapshots) > 0 {
			for i := len(datedSnapshots) - 1; i >= int(*policy.Spec.Retention); i-- {
				glog.Infof("Deleting expired snapshot %s", datedSnapshots[i].Snapshot.GetObjectMeta().GetName())
				strat.deleteSnapshot(datedSnapshots[i].Snapshot)
			}
		}
	}
}
