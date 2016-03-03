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

package app

// This file exists to force the desired provider implementations to be linked.
// This should probably be part of some configuration fed into the build for a
// given binary target.
import (
	_ "github.com/skyrings/skyring-common/dbprovider/mongodb"
	_ "github.com/skyrings/skyring-common/monitoring/graphitemanager"
	_ "github.com/skyrings/skyring-common/monitoring/influxdbmanager"
	_ "github.com/skyrings/skyring-common/provisioner/cephinstaller"
	_ "github.com/skyrings/skyring/authprovider/ldap"
	_ "github.com/skyrings/skyring/authprovider/local"
	_ "github.com/skyrings/skyring/nodemanager/saltnodemanager"
)
