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

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SnapshotPolicyResourcePlural - Plural form of SnapshotPolicy
	SnapshotPolicyResourcePlural = "snapshotpolicies"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotPolicy is a specification for a SnapshotPolicy resource
type SnapshotPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotPolicySpec   `json:"spec"`
	Status SnapshotPolicyStatus `json:"status"`
}

// Strategy - The name for the operation which will undertake the snapshotting
type Strategy struct {
	Name string `json:"name"`
}

// SnapshotPolicySpec is the spec for a SnapshotPolicy resource
type SnapshotPolicySpec struct {
	PVCNames []string `json:"claims"`
	Unit     string   `json:"unit,omitempty"`
	Period   *int32   `json:"period,omitempty"`
	// Duration - Calculated duration between periods, not easily expressed in yaml form
	Duration  time.Duration `json:"-"`
	Retention *int32        `json:"retention,omitempty"`
	Strategy  Strategy      `json:"strategy,omitempty"`
}

// SnapshotPolicyStatus is the status for a SnapshotPolicy resource
type SnapshotPolicyStatus struct {
	LastSnapshotTime string `json:"lastSnapshotTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotPolicyList is a list of SnapshotPolicy resources
type SnapshotPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SnapshotPolicy `json:"items"`
}
