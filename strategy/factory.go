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

	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
)

type StrategyName string

const (
	InUse StrategyName = "inuse"
)

func CreateStrategy(config *rest.RESTClient, pvcLister corelisters.PersistentVolumeClaimLister, name StrategyName) (Strategy, error) {

	switch name {
	case InUse:
		return &inUseStragey{
				client:    config,
				pvcLister: pvcLister,
			},
			nil
		// case mysql
		// case postgres
	}
	return nil, fmt.Errorf("Strategy %s is not a valid strategy!", name)
}
