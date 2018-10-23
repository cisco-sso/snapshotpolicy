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
	v1alpha1 "github.com/cisco-sso/snapshotpolicy/pkg/apis/snapshotpolicy/v1alpha1"
)

const (
	//EventRun - Name of the event for the strategy run result
	EventRun = "StrategyRun"
)

// Strategy - Defines a process for creating a snapshot.
type Strategy interface {
	// TODO: Add context for cancellation
	Run(v1alpha1.SnapshotPolicy) (*v1alpha1.SnapshotPolicyStatus, error)
}
