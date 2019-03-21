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

package endpoints

import (
	"fmt"
	"github.com/jmespath/go-jmespath"
	"strings"
)

// LocalGlobalResolver is global resolver
type LocalGlobalResolver struct{}

// TryResolve returns endpoint
func (resolver *LocalGlobalResolver) TryResolve(param *ResolveParam) (endpoint string, support bool, err error) {
	// get the global endpoints configs
	endpointExpression := fmt.Sprintf("products[?code=='%s'].global_endpoint", strings.ToLower(param.Product))
	endpointData, err := jmespath.Search(endpointExpression, getEndpointConfigData())
	if err == nil && endpointData != nil && len(endpointData.([]interface{})) > 0 {
		endpoint = endpointData.([]interface{})[0].(string)
		support = len(endpoint) > 0
		return endpoint, support, nil
	}
	support = false
	return
}
