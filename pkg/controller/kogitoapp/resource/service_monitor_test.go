// Copyright 2019 Red Hat, Inc. and/or its affiliates
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

package resource

import (
	"reflect"
	"testing"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	monfake "github.com/coreos/prometheus-operator/pkg/client/versioned/fake"
	v1 "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"

	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client"

	dockerv10 "github.com/openshift/api/image/docker10"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

func TestNewServiceMonitor(t *testing.T) {
	port := intstr.FromInt(8080)

	type args struct {
		kogitoApp   *v1alpha1.KogitoApp
		dockerImage *dockerv10.DockerImage
		service     *corev1.Service
		client      *client.Client
	}
	tests := []struct {
		name    string
		args    args
		want    *monv1.ServiceMonitor
		wantErr bool
	}{
		{
			"TestNewServiceMonitor",
			args{
				&v1alpha1.KogitoApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
				&dockerv10.DockerImage{
					Config: &dockerv10.DockerConfig{
						Labels: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/path":   "/ms",
							"prometheus.io/port":   "8080",
							"prometheus.io/scheme": "https",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"test": "test",
						},
					},
				},
				&client.Client{
					ControlCli:    fake.NewFakeClient(),
					PrometheusCli: monfake.NewSimpleClientset().MonitoringV1(),
				},
			},
			&monv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceMonitor",
					APIVersion: "monitoring.coreos.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: defaultAnnotations,
				},
				Spec: monv1.ServiceMonitorSpec{
					NamespaceSelector: monv1.NamespaceSelector{
						MatchNames: []string{
							"test",
						},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					Endpoints: []monv1.Endpoint{
						{
							TargetPort: &port,
							Path:       "/ms",
							Scheme:     "https",
						},
					},
				},
			},
			false,
		},

		{
			"TestNewServiceMonitorDefault",
			args{
				&v1alpha1.KogitoApp{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				},
				&dockerv10.DockerImage{
					Config: &dockerv10.DockerConfig{
						Labels: map[string]string{
							"prometheus.io/scrape": "true",
						},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"test": "test",
						},
					},
				},
				&client.Client{
					ControlCli:    fake.NewFakeClient(),
					PrometheusCli: monfake.NewSimpleClientset().MonitoringV1(),
				},
			},
			&monv1.ServiceMonitor{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceMonitor",
					APIVersion: "monitoring.coreos.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"app": "test",
					},
					Annotations: defaultAnnotations,
				},
				Spec: monv1.ServiceMonitorSpec{
					NamespaceSelector: monv1.NamespaceSelector{
						MatchNames: []string{
							"test",
						},
					},
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					Endpoints: []monv1.Endpoint{
						{
							Port:   "http",
							Path:   "/metrics",
							Scheme: "http",
						},
					},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewServiceMonitor(tt.args.kogitoApp, tt.args.dockerImage, tt.args.service, tt.args.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServiceMonitor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewServiceMonitor() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isPrometheusOperatorReady(t *testing.T) {
	type args struct {
		kogitoApp *v1alpha1.KogitoApp
		client    *client.Client
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			"PrometheusOperatorReady",
			args{
				&v1alpha1.KogitoApp{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
					},
				},
				&client.Client{
					ControlCli:    fake.NewFakeClient(),
					PrometheusCli: monfake.NewSimpleClientset().MonitoringV1(),
				},
			},
			true,
			false,
		},
		{
			"PrometheusOperatorNotReady",
			args{
				&v1alpha1.KogitoApp{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "test",
					},
				},
				&client.Client{
					ControlCli:    fake.NewFakeClient(),
					PrometheusCli: &mockMonitoringClient{},
				},
			},
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isPrometheusOperatorReady(tt.args.kogitoApp, tt.args.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("isPrometheusOperatorReady() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("isPrometheusOperatorReady() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockMonitoringClient struct{}

func (m *mockMonitoringClient) RESTClient() rest.Interface {
	panic("No Implementation")
}

func (m *mockMonitoringClient) Alertmanagers(namespace string) v1.AlertmanagerInterface {
	panic("No Implementation")
}

func (m *mockMonitoringClient) PodMonitors(namespace string) v1.PodMonitorInterface {
	panic("No Implementation")
}

func (m *mockMonitoringClient) Prometheuses(namespace string) v1.PrometheusInterface {
	panic("No Implementation")
}

func (m *mockMonitoringClient) PrometheusRules(namespace string) v1.PrometheusRuleInterface {
	panic("No Implementation")
}

func (m *mockMonitoringClient) ServiceMonitors(namespace string) v1.ServiceMonitorInterface {
	return &mockServiceMonitorClient{}
}

type mockServiceMonitorClient struct{}

func (m *mockServiceMonitorClient) Create(*monv1.ServiceMonitor) (*monv1.ServiceMonitor, error) {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) Update(*monv1.ServiceMonitor) (*monv1.ServiceMonitor, error) {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) Delete(name string, options *metav1.DeleteOptions) error {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) Get(name string, options metav1.GetOptions) (*monv1.ServiceMonitor, error) {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) List(opts metav1.ListOptions) (*monv1.ServiceMonitorList, error) {
	return nil, &errors.StatusError{
		ErrStatus: metav1.Status{
			Reason: metav1.StatusReasonNotFound,
		},
	}
}

func (m *mockServiceMonitorClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	panic("No Implementation")
}

func (m *mockServiceMonitorClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *monv1.ServiceMonitor, err error) {
	panic("No Implementation")
}
