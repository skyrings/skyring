/*Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package models

type RpcRequest struct {
	RpcRequestVars map[string]string `json:"rpcrequestvars"`
	RpcRequestData []byte            `json:"rpcrequestdata"`
}

type RpcResponse struct {
	Status RpcResponseStatus `json:"status"`
	Data   RpcResponseData   `json:"data"`
}

type RpcResponseStatus struct {
	StatusCode    int    `json:"statuscode"`
	StatusMessage string `json:"statusmessage"`
}

type RpcResponseData struct {
	RequestId string `json:"requestid"`
	Result    []byte `json:"result"`
}

/**
 * Below generic RPC methods should be implemented by storage providers
 * to work with skyring.
 * Input parameters:
 * - req : RpcRequest instance
 * Output parameter:
 * - resp : pointer to RpcResponse instance
 * Returns:
 * - error
 */
type StorageProviderInterface interface {
	CreateCluster(req RpcRequest, resp *RpcResponse) error
}
