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

package services

import (
	"github.com/RHsyseng/operator-utils/pkg/resource"
	"github.com/RHsyseng/operator-utils/pkg/resource/compare"
	"github.com/RHsyseng/operator-utils/pkg/resource/read"
	keycloakv1alpha1 "github.com/keycloak/keycloak-operator/pkg/apis/keycloak/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	kafkabetav1 "github.com/kiegroup/kogito-cloud-operator/pkg/apis/kafka/v1beta1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/framework"
	"github.com/kiegroup/kogito-cloud-operator/pkg/infrastructure"
	imgv1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
)

const (
	enablePersistenceEnvKey = "ENABLE_PERSISTENCE"
)

// createRequiredResources creates the required resources given the KogitoService instance
func (s *serviceDeployer) createRequiredResources(instance v1alpha1.KogitoService) (resources map[reflect.Type][]resource.KubernetesResource, err error) {
	resources = make(map[reflect.Type][]resource.KubernetesResource)
	imageHandler := newImageHandler(instance, s.definition.DefaultImageName, s.client)
	if imageHandler.hasImageStream() {
		resources[reflect.TypeOf(imgv1.ImageStream{})] = []resource.KubernetesResource{imageHandler.imageStream}
	}

	// we only create the rest of the resources once we have a resolvable image
	// or if the deployment is already there, we don't want to delete it :)
	if image, err := s.getKogitoServiceImage(imageHandler, instance); err != nil {
		return resources, err
	} else if len(image) > 0 {
		deployment := createRequiredDeployment(instance, image, s.definition)
		if err = s.applyDeploymentCustomizations(deployment, instance, imageHandler); err != nil {
			return resources, err
		}

		service := createRequiredService(instance, deployment)

		if s.definition.infinispanAware {
			if err = s.applyInfinispanConfigurations(deployment, instance); err != nil {
				return resources, err
			}
		}
		if s.definition.kafkaAware {
			if err = s.applyKafkaConfigurations(deployment, instance); err != nil {
				return resources, err
			}
		}

		if s.definition.keycloakAware {
			if instance.GetSpec().(v1alpha1.KeycloakAware).GetKeycloakProperties().UseKogitoInfra &&
				infrastructure.IsKeycloakAvailable(s.client) {
				keycloakClient := s.createKeycloakClient(instance)
				keycloakUser := s.createKeycloakUser(instance)
				resources[reflect.TypeOf(keycloakv1alpha1.KeycloakClient{})] = []resource.KubernetesResource{keycloakClient}
				resources[reflect.TypeOf(keycloakv1alpha1.KeycloakUser{})] = []resource.KubernetesResource{keycloakUser}
			}

			if err = s.applyKeycloakConfigurations(deployment, instance); err != nil {
				return resources, err
			}
		}

		resources[reflect.TypeOf(appsv1.Deployment{})] = []resource.KubernetesResource{deployment}
		resources[reflect.TypeOf(corev1.Service{})] = []resource.KubernetesResource{service}
		if s.client.IsOpenshift() {
			resources[reflect.TypeOf(routev1.Route{})] = []resource.KubernetesResource{createRequiredRoute(instance, service)}
		}
	}

	return
}

func (s *serviceDeployer) applyDeploymentCustomizations(deployment *appsv1.Deployment, instance v1alpha1.KogitoService, imageHandler *imageHandler) error {
	if imageHandler.hasImageStream() {
		key, value := framework.ResolveImageStreamTriggerAnnotation(imageHandler.resolveImageNameTag(), instance.GetName())
		deployment.Annotations = map[string]string{key: value}
	}
	if s.definition.OnDeploymentCreate != nil {
		if err := s.definition.OnDeploymentCreate(deployment, instance); err != nil {
			return err
		}
	}
	return nil
}

func (s *serviceDeployer) getKogitoServiceImage(imageHandler *imageHandler, instance v1alpha1.KogitoService) (string, error) {
	image, err := imageHandler.resolveImage()
	if err != nil {
		return "", err
	}
	if len(image) > 0 {
		return image, nil
	}
	deploymentDeployed := &appsv1.Deployment{ObjectMeta: v1.ObjectMeta{Name: instance.GetName(), Namespace: instance.GetNamespace()}}
	if exists, err := kubernetes.ResourceC(s.client).Fetch(deploymentDeployed); err != nil {
		return "", err
	} else if !exists {
		return "", nil
	}
	if len(deploymentDeployed.Spec.Template.Spec.Containers) > 0 {
		return deploymentDeployed.Spec.Template.Spec.Containers[0].Image, nil
	}
	return "", nil
}

func (s *serviceDeployer) applyInfinispanConfigurations(deployment *appsv1.Deployment, instance v1alpha1.KogitoService) error {
	var infinispanSecret *corev1.Secret
	infinispanAware := instance.GetSpec().(v1alpha1.InfinispanAware)
	infinispanSecret, err := infrastructure.FetchInfinispanCredentials(infinispanAware, instance.GetNamespace(), s.client)
	if err != nil {
		return err
	}
	infrastructure.SetInfinispanVariables(
		*infinispanAware.GetInfinispanProperties(),
		infinispanSecret,
		&deployment.Spec.Template.Spec.Containers[0])

	if infinispanAware.GetInfinispanProperties().UseKogitoInfra || len(infinispanAware.GetInfinispanProperties().URI) > 0 {
		framework.SetEnvVar(enablePersistenceEnvKey, "true", &deployment.Spec.Template.Spec.Containers[0])
	}
	return nil
}

func (s *serviceDeployer) applyKafkaConfigurations(deployment *appsv1.Deployment, instance v1alpha1.KogitoService) error {
	URI, err := getKafkaServerURI(*instance.GetSpec().(v1alpha1.KafkaAware).GetKafkaProperties(), s.getNamespace(), s.client)
	if err != nil {
		return err
	}

	if len(URI) > 0 {
		for _, kafkaTopic := range s.definition.KafkaTopics {
			framework.SetEnvVar(fromKafkaTopicToQuarkusEnvVar(kafkaTopic), URI, &deployment.Spec.Template.Spec.Containers[0])
		}
		framework.SetEnvVar(quarkusBootstrapEnvVar, URI, &deployment.Spec.Template.Spec.Containers[0])
	}

	return nil
}

func (s *serviceDeployer) applyKeycloakConfigurations(deployment *appsv1.Deployment, instance v1alpha1.KogitoService) error {
	keycloakAware := instance.GetSpec().(v1alpha1.KeycloakAware)
	if keycloakAware.GetKeycloakProperties().UseKogitoInfra && infrastructure.IsKeycloakAvailable(s.client) {
		setKeycloakVariables(keycloakAware.GetKeycloakProperties(), s.definition.KeycloakClientUser, &deployment.Spec.Template.Spec.Containers[0])
		setKeycloakCertificateVolume(&deployment.Spec.Template.Spec, 0)
	}
	return nil
}

func (s *serviceDeployer) createKeycloakClient(instance v1alpha1.KogitoService) (keycloakClient resource.KubernetesResource) {
	if !infrastructure.IsKeycloakAvailable(s.client) {
		return
	}
	keycloakClient = newKeycloakClient(s.getNamespace(), instance.GetSpec().(v1alpha1.KeycloakAware).GetKeycloakProperties().Labels, s.definition.KeycloakClientUser)
	return
}

func (s *serviceDeployer) createKeycloakUser(instance v1alpha1.KogitoService) (keycloakUser resource.KubernetesResource) {
	if !infrastructure.IsKeycloakAvailable(s.client) {
		return
	}
	keycloakUser = newKeycloakUser(s.getNamespace(), instance.GetSpec().(v1alpha1.KeycloakAware).GetKeycloakProperties().Labels, s.definition.KeycloakClientUser)
	return
}

// getDeployedResources gets the deployed resources in the cluster owned by the given instance
func (s *serviceDeployer) getDeployedResources(instance v1alpha1.KogitoService) (resources map[reflect.Type][]resource.KubernetesResource, err error) {
	reader := read.New(s.client.ControlCli).WithNamespace(instance.GetNamespace()).WithOwnerObject(instance)
	var objectTypes []runtime.Object
	if s.client.IsOpenshift() {
		objectTypes = []runtime.Object{&appsv1.DeploymentList{}, &corev1.ServiceList{}, &routev1.RouteList{}, &imgv1.ImageStreamList{}}
	} else {
		objectTypes = []runtime.Object{&appsv1.DeploymentList{}, &corev1.ServiceList{}}
	}

	if infrastructure.IsStrimziAvailable(s.client) {
		objectTypes = append(objectTypes, &kafkabetav1.KafkaTopicList{})
	}
	if infrastructure.IsKeycloakAvailable(s.client) {
		objectTypes = append(objectTypes, &keycloakv1alpha1.KeycloakClientList{}, &keycloakv1alpha1.KeycloakUserList{})
	}
	return reader.ListAll(objectTypes...)
}

// getComparator gets the comparator for the owned resources
func (s *serviceDeployer) getComparator() compare.MapComparator {
	resourceComparator := compare.DefaultComparator()

	resourceComparator.SetComparator(
		framework.NewComparatorBuilder().
			WithType(reflect.TypeOf(appsv1.Deployment{})).
			UseDefaultComparator().
			Build())

	resourceComparator.SetComparator(
		framework.NewComparatorBuilder().
			WithType(reflect.TypeOf(corev1.Service{})).
			UseDefaultComparator().
			Build())

	resourceComparator.SetComparator(
		framework.NewComparatorBuilder().
			WithType(reflect.TypeOf(routev1.Route{})).
			UseDefaultComparator().
			WithCustomComparator(framework.CreateRouteComparator()).
			Build())

	resourceComparator.SetComparator(
		framework.NewComparatorBuilder().
			WithType(reflect.TypeOf(imgv1.ImageStream{})).
			UseDefaultComparator().
			WithCustomComparator(framework.CreateImageStreamComparator()).
			Build())

	return compare.MapComparator{Comparator: resourceComparator}
}
