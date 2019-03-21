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

package ecs

import (
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/alicloud/alibaba-cloud-sdk-go/sdk/requests"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/alicloud/alibaba-cloud-sdk-go/sdk/responses"
)

// DescribeInstanceTypes invokes the ecs.DescribeInstanceTypes API synchronously
// api document: https://help.aliyun.com/api/ecs/describeinstancetypes.html
func (client *Client) DescribeInstanceTypes(request *DescribeInstanceTypesRequest) (response *DescribeInstanceTypesResponse, err error) {
	response = CreateDescribeInstanceTypesResponse()
	err = client.DoAction(request, response)
	return
}

// DescribeInstanceTypesWithChan invokes the ecs.DescribeInstanceTypes API asynchronously
// api document: https://help.aliyun.com/api/ecs/describeinstancetypes.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) DescribeInstanceTypesWithChan(request *DescribeInstanceTypesRequest) (<-chan *DescribeInstanceTypesResponse, <-chan error) {
	responseChan := make(chan *DescribeInstanceTypesResponse, 1)
	errChan := make(chan error, 1)
	err := client.AddAsyncTask(func() {
		defer close(responseChan)
		defer close(errChan)
		response, err := client.DescribeInstanceTypes(request)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
	})
	if err != nil {
		errChan <- err
		close(responseChan)
		close(errChan)
	}
	return responseChan, errChan
}

// DescribeInstanceTypesWithCallback invokes the ecs.DescribeInstanceTypes API asynchronously
// api document: https://help.aliyun.com/api/ecs/describeinstancetypes.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) DescribeInstanceTypesWithCallback(request *DescribeInstanceTypesRequest, callback func(response *DescribeInstanceTypesResponse, err error)) <-chan int {
	result := make(chan int, 1)
	err := client.AddAsyncTask(func() {
		var response *DescribeInstanceTypesResponse
		var err error
		defer close(result)
		response, err = client.DescribeInstanceTypes(request)
		callback(response, err)
		result <- 1
	})
	if err != nil {
		defer close(result)
		callback(nil, err)
		result <- 0
	}
	return result
}

// DescribeInstanceTypesRequest is the request struct for api DescribeInstanceTypes
type DescribeInstanceTypesRequest struct {
	*requests.RpcRequest
	ResourceOwnerId      requests.Integer `position:"Query" name:"ResourceOwnerId"`
	ResourceOwnerAccount string           `position:"Query" name:"ResourceOwnerAccount"`
	OwnerAccount         string           `position:"Query" name:"OwnerAccount"`
	InstanceTypeFamily   string           `position:"Query" name:"InstanceTypeFamily"`
	OwnerId              requests.Integer `position:"Query" name:"OwnerId"`
}

// DescribeInstanceTypesResponse is the response struct for api DescribeInstanceTypes
type DescribeInstanceTypesResponse struct {
	*responses.BaseResponse
	RequestId     string                               `json:"RequestId" xml:"RequestId"`
	InstanceTypes InstanceTypesInDescribeInstanceTypes `json:"InstanceTypes" xml:"InstanceTypes"`
}

// InstanceTypesInDescribeInstanceTypes is a nested struct in ecs response
type InstanceTypesInDescribeInstanceTypes struct {
	InstanceType []InstanceType `json:"InstanceType" xml:"InstanceType"`
}

// InstanceType is a nested struct in ecs response
type InstanceType struct {
	MemorySize           float64 `json:"MemorySize" xml:"MemorySize"`
	InstancePpsRx        int     `json:"InstancePpsRx" xml:"InstancePpsRx"`
	CpuCoreCount         int     `json:"CpuCoreCount" xml:"CpuCoreCount"`
	Cores                int     `json:"Cores" xml:"Cores"`
	Memory               int     `json:"Memory" xml:"Memory"`
	InstanceTypeId       string  `json:"InstanceTypeId" xml:"InstanceTypeId"`
	InstanceBandwidthRx  int     `json:"InstanceBandwidthRx" xml:"InstanceBandwidthRx"`
	InstanceType         string  `json:"InstanceType" xml:"InstanceType"`
	BaselineCredit       int     `json:"BaselineCredit" xml:"BaselineCredit"`
	EniQuantity          int     `json:"EniQuantity" xml:"EniQuantity"`
	Generation           string  `json:"Generation" xml:"Generation"`
	GPUAmount            int     `json:"GPUAmount" xml:"GPUAmount"`
	SupportIoOptimized   string  `json:"SupportIoOptimized" xml:"SupportIoOptimized"`
	InstanceTypeFamily   string  `json:"InstanceTypeFamily" xml:"InstanceTypeFamily"`
	InitialCredit        int     `json:"InitialCredit" xml:"InitialCredit"`
	InstancePpsTx        int     `json:"InstancePpsTx" xml:"InstancePpsTx"`
	LocalStorageAmount   int     `json:"LocalStorageAmount" xml:"LocalStorageAmount"`
	InstanceFamilyLevel  string  `json:"InstanceFamilyLevel" xml:"InstanceFamilyLevel"`
	LocalStorageCapacity int     `json:"LocalStorageCapacity" xml:"LocalStorageCapacity"`
	GPUSpec              string  `json:"GPUSpec" xml:"GPUSpec"`
	LocalStorageCategory string  `json:"LocalStorageCategory" xml:"LocalStorageCategory"`
	InstanceBandwidthTx  int     `json:"InstanceBandwidthTx" xml:"InstanceBandwidthTx"`
}

// CreateDescribeInstanceTypesRequest creates a request to invoke DescribeInstanceTypes API
func CreateDescribeInstanceTypesRequest() (request *DescribeInstanceTypesRequest) {
	request = &DescribeInstanceTypesRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("Ecs", "2014-05-26", "DescribeInstanceTypes", "ecs", "openAPI")
	return
}

// CreateDescribeInstanceTypesResponse creates a response to parse from DescribeInstanceTypes response
func CreateDescribeInstanceTypesResponse() (response *DescribeInstanceTypesResponse) {
	response = &DescribeInstanceTypesResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}
