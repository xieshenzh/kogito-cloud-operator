// Copyright 2020 Red Hat, Inc. and/or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package infrastructure

import (
	"fmt"
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"time"
)

const (
	serviceURLPattern = "https://%s.%s.svc:%d"
)

// DeployKeycloakWithKogitoInfra deploys KogitoInfra with Keycloak enabled
// returns update = true if the instance needs to be updated, a duration for requeue and error != nil if something goes wrong
func DeployKeycloakWithKogitoInfra(instance v1alpha1.KeycloakAware, namespace string, cli *client.Client) (update bool, requeueAfter time.Duration, err error) {
	if instance == nil {
		return false, 0, nil
	}

	// Overrides any parameters not set
	if instance.GetKeycloakProperties().UseKogitoInfra {
		// ensure infra
		infra, ready, err := EnsureKogitoInfra(namespace, cli).WithKeycloak().Apply()
		if err != nil {
			return false, 0, err
		}

		log.Debugf("Checking KogitoInfra status to make sure we are ready to use Keycloak. Status are: %s", infra.Status.Keycloak)
		if ready {
			keycloak, route, err := GetKeycloakProperties(cli, infra)
			if err != nil {
				return false, 0, err
			}
			keycloakURL, err := getKeycloakServiceURL(route, infra.Namespace, cli)
			if err != nil {
				return false, 0, err
			}
			keycloakRealm, realmName, keycloakRealmLabels, err := GetKeycloakRealmProperties(cli, infra)
			if err != nil {
				return false, 0, err
			}
			if len(keycloakURL) > 0 && len(realmName) > 0 && len(keycloakRealmLabels) > 0 {
				if instance.GetKeycloakProperties().Keycloak == keycloak &&
					instance.GetKeycloakProperties().KeycloakRealm == keycloakRealm &&
					instance.GetKeycloakProperties().AuthServerURL == keycloakURL &&
					instance.GetKeycloakProperties().RealmName == realmName &&
					reflect.DeepEqual(instance.GetKeycloakProperties().Labels, keycloakRealmLabels) {
					return false, 0, nil
				}

				log.Debug("Looks ok, we are ready to use Keycloak!")
				instance.SetKeycloakProperties(v1alpha1.KeycloakConnectionProperties{
					Keycloak:      keycloak,
					KeycloakRealm: keycloakRealm,
					AuthServerURL: keycloakURL,
					RealmName:     realmName,
					Labels:        keycloakRealmLabels,
				})

				return true, 0, nil
			}
		}
		log.Debug("KogitoInfra is not ready, requeue")
		// waiting for infinispan deployment
		return false, time.Second * 10, nil
	}

	// Ensure default values
	if instance.AreKeycloakPropertiesBlank() {
		instance.SetKeycloakProperties(v1alpha1.KeycloakConnectionProperties{
			UseKogitoInfra: true,
		})
		return true, 0, nil
	}

	return false, 0, nil
}

// getKeycloakServiceURL resolves the internal URL of the keycloak service
func getKeycloakServiceURL(routeName, namespace string, cli *client.Client) (URL string, err error) {
	route := &routev1.Route{}
	if exists, err := kubernetes.ResourceC(cli).FetchWithKey(types.NamespacedName{Name: routeName, Namespace: namespace}, route); err != nil {
		return "", err
	} else if exists && route.Spec.To.Kind == "Service" {
		serviceName := route.Spec.To.Name
		if route.Spec.Port.TargetPort.Type == intstr.String {
			service := &corev1.Service{}
			if exists, err := kubernetes.ResourceC(cli).FetchWithKey(types.NamespacedName{Name: serviceName, Namespace: namespace}, service); err != nil {
				return "", err
			} else if exists {
				for _, port := range service.Spec.Ports {
					if port.Name == route.Spec.Port.TargetPort.StrVal {
						return fmt.Sprintf(serviceURLPattern, serviceName, namespace, port.Port), nil
					}
				}
			}
		} else {
			return fmt.Sprintf(serviceURLPattern, serviceName, namespace, route.Spec.Port.TargetPort.IntVal), nil
		}
	}
	return "", nil
}
