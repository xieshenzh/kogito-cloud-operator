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
	keycloakv1alpha1 "github.com/keycloak/keycloak-operator/pkg/apis/keycloak/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	profileEnv    = "QUARKUS_PROFILE"
	authServerEnv = "QUARKUS_OIDC_AUTH_SERVER_URL"
	clientEnv     = "QUARKUS_OIDC_CLIENT_ID"
	secretEnv     = "QUARKUS_OIDC_CREDENTIALS_SECRET"

	securityProfile = "keycloak"
	authPath        = "/auth/realms/"

	certificateAliasEnv    = "KEYCLOAK_CERT_ALIAS"
	certificatePasswordEnv = "KEYCLOAK_CERT_PWD"
	certificatePathEnv     = "KEYCLOAK_CERT_FILE"

	certificateFile     = "service-ca.crt"
	certificateAlias    = "keycloak"
	certificatePassword = "keycloak"
	certificatePath     = "/etc/ssl/certs/" + certificateFile

	volumeName = "keycloak"

	// CertificateConfigMapName TODO
	CertificateConfigMapName = "keycloak-certificate"
)

func newKeycloakUser(namespace string, realmLabels map[string]string, keycloakClientuser *KeycloakClientUserDefinition) *keycloakv1alpha1.KeycloakUser {
	return &keycloakv1alpha1.KeycloakUser{
		ObjectMeta: v1.ObjectMeta{
			Name:      keycloakClientuser.KeycloakUserName,
			Namespace: namespace,
		},
		Spec: keycloakv1alpha1.KeycloakUserSpec{
			RealmSelector: &v1.LabelSelector{
				MatchLabels: realmLabels,
			},
			User: keycloakv1alpha1.KeycloakAPIUser{
				UserName:      keycloakClientuser.UserName,
				Enabled:       true,
				EmailVerified: false,
				RealmRoles: []string{
					keycloakClientuser.UserRole,
				},
				Credentials: []keycloakv1alpha1.KeycloakCredential{
					{
						Type:      "password",
						Value:     keycloakClientuser.Password,
						Temporary: false,
					},
				},
			},
		},
	}
}

func newKeycloakClient(namespace string, realmLabels map[string]string, keycloakClientuser *KeycloakClientUserDefinition) *keycloakv1alpha1.KeycloakClient {
	return &keycloakv1alpha1.KeycloakClient{
		ObjectMeta: v1.ObjectMeta{
			Name:      keycloakClientuser.KeycloakClientName,
			Namespace: namespace,
		},
		Spec: keycloakv1alpha1.KeycloakClientSpec{
			RealmSelector: &v1.LabelSelector{
				MatchLabels: realmLabels,
			},
			Client: &keycloakv1alpha1.KeycloakAPIClient{
				ClientID:                  keycloakClientuser.ClientID,
				PublicClient:              false,
				BearerOnly:                false,
				ClientAuthenticatorType:   keycloakClientuser.ClientAuthenticatorType,
				Secret:                    keycloakClientuser.Secret,
				ServiceAccountsEnabled:    true,
				DirectAccessGrantsEnabled: true,
				RedirectUris:              []string{"*"},
				WebOrigins:                []string{"*"},
				Access: map[string]bool{
					"view":      true,
					"configure": true,
					"manage":    true,
				},
			},
		},
	}
}

// setKeycloakVariables binds Keycloak properties in the container.
func setKeycloakVariables(keycloakProps *v1alpha1.KeycloakConnectionProperties, keycloakClientUser *KeycloakClientUserDefinition, container *corev1.Container) {
	if len(keycloakProps.AuthServerURL) > 0 && len(keycloakProps.RealmName) > 0 {
		framework.SetEnvVar(profileEnv, securityProfile, container)
		framework.SetEnvVar(authServerEnv, keycloakProps.AuthServerURL+authPath+keycloakProps.RealmName, container)
		framework.SetEnvVar(clientEnv, keycloakClientUser.ClientID, container)
		framework.SetEnvVar(secretEnv, keycloakClientUser.Secret, container)
		framework.SetEnvVar(certificateAliasEnv, certificateAlias, container)
		framework.SetEnvVar(certificatePasswordEnv, certificatePassword, container)
		framework.SetEnvVar(certificatePathEnv, certificatePath, container)
	}
}

// setKeycloakCertificateVolume binds certificate in the container
func setKeycloakCertificateVolume(spec *corev1.PodSpec, containerIndex int) {
	spec.Containers[containerIndex].VolumeMounts = append(spec.Containers[containerIndex].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: certificatePath,
		SubPath:   certificateFile,
	})

	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: CertificateConfigMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  certificateFile,
						Path: certificateFile,
					},
				},
			},
		},
	})
}
