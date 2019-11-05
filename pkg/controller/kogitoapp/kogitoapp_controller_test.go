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

package kogitoapp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	cachev1 "sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kiegroup/kogito-cloud-operator/pkg/apis/app/v1alpha1"
	kogitoclient "github.com/kiegroup/kogito-cloud-operator/pkg/client"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/kubernetes"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/meta"
	"github.com/kiegroup/kogito-cloud-operator/pkg/client/openshift"
	kogitores "github.com/kiegroup/kogito-cloud-operator/pkg/controller/kogitoapp/resource"
	"github.com/kiegroup/kogito-cloud-operator/pkg/controller/kogitoapp/shared"
	"github.com/kiegroup/kogito-cloud-operator/pkg/controller/kogitoapp/status"
	"github.com/kiegroup/kogito-cloud-operator/pkg/test"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	monfake "github.com/coreos/prometheus-operator/pkg/client/versioned/fake"

	appsv1 "github.com/openshift/api/apps/v1"
	buildv1 "github.com/openshift/api/build/v1"
	dockerv10 "github.com/openshift/api/image/docker10"
	imgv1 "github.com/openshift/api/image/v1"
	routev1 "github.com/openshift/api/route/v1"
	buildfake "github.com/openshift/client-go/build/clientset/versioned/fake"
	imgfake "github.com/openshift/client-go/image/clientset/versioned/fake"

	"github.com/stretchr/testify/assert"
)

var (
	cpuResource    = v1alpha1.ResourceCPU
	memoryResource = v1alpha1.ResourceMemory
	cpuValue       = "1"
	cr             = v1alpha1.KogitoApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "test",
		},
		Spec: v1alpha1.KogitoAppSpec{
			Resources: v1alpha1.Resources{
				Limits: []v1alpha1.ResourceMap{
					{
						Resource: cpuResource,
						Value:    cpuValue,
					},
				},
			},
		},
	}
)

func TestNewContainerWithResource(t *testing.T) {
	container := corev1.Container{
		Name:            cr.Name,
		Env:             shared.FromEnvToEnvVar(cr.Spec.Env),
		Resources:       shared.FromResourcesToResourcesRequirements(cr.Spec.Resources),
		ImagePullPolicy: corev1.PullAlways,
	}
	assert.NotNil(t, container)
	cpuQty := resource.MustParse(cpuValue)
	assert.Equal(t, container.Resources.Limits.Cpu(), &cpuQty)
	assert.Equal(t, container.Resources.Requests.Cpu(), &resource.Quantity{Format: resource.DecimalSI})
}

func createFakeKogitoApp() *v1alpha1.KogitoApp {
	gitURL := "https://github.com/kiegroup/kogito-examples/"
	kogitoapp := &cr
	kogitoapp.Spec.Build = &v1alpha1.KogitoAppBuildObject{
		ImageS2I: v1alpha1.Image{
			ImageStreamTag: "0.4.0",
		},
		ImageRuntime: v1alpha1.Image{
			ImageStreamTag: "0.4.0",
		},
		GitSource: &v1alpha1.GitSource{
			URI:        &gitURL,
			ContextDir: "jbpm-quarkus-example",
		},
	}

	return kogitoapp
}

func createFakeImages(kogitoAppName string) []runtime.Object {
	dockerImageRaw, _ := json.Marshal(&dockerv10.DockerImage{
		Config: &dockerv10.DockerConfig{
			Labels: map[string]string{
				openshift.ImageLabelForExposeServices: "8080:http",
			},
		},
	})

	isTag := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:latest", kogitoAppName),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}

	isTagBuild := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:latest", kogitoAppName+"-build"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}

	image1 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoQuarkusUbi8Image, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}
	image2 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoQuarkusJVMUbi8Image, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}
	image3 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoQuarkusUbi8s2iImage, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}
	image4 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoSpringbootUbi8Image, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}
	image5 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoSpringbootUbi8s2iImage, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}
	image6 := imgv1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s:%s", kogitores.KogitoDataIndexImage, "0.4.0"),
			Namespace: "test",
		},
		Image: imgv1.Image{
			DockerImageMetadata: runtime.RawExtension{
				Raw: dockerImageRaw,
			},
		},
	}

	return []runtime.Object{&isTag, &isTagBuild, &image1, &image2, &image3, &image4, &image5, &image6}
}

func setupSchema(kogitoapp *v1alpha1.KogitoApp) *runtime.Scheme {
	// add to schemas to avoid: "failed to add object to fake client"
	s := meta.GetRegisteredSchema()
	s.AddKnownTypes(v1alpha1.SchemeGroupVersion, kogitoapp, &v1alpha1.KogitoAppList{})
	s.AddKnownTypes(appsv1.GroupVersion, &appsv1.DeploymentConfig{}, &appsv1.DeploymentConfigList{})
	s.AddKnownTypes(buildv1.GroupVersion, &buildv1.BuildConfig{}, &buildv1.BuildConfigList{})
	s.AddKnownTypes(routev1.GroupVersion, &routev1.Route{}, &routev1.RouteList{})
	s.AddKnownTypes(imgv1.GroupVersion, &imgv1.ImageStreamTag{}, &imgv1.ImageStream{}, &imgv1.ImageStreamList{})
	s.AddKnownTypes(monv1.SchemeGroupVersion, &monv1.ServiceMonitor{}, &monv1.ServiceMonitorList{})
	return s
}

func TestKogitoAppWithResource(t *testing.T) {
	kogitoapp := createFakeKogitoApp()
	images := createFakeImages(kogitoapp.Name)
	objs := []runtime.Object{kogitoapp}
	s := setupSchema(kogitoapp)
	// Create a fake client to mock API calls.
	cli := fake.NewFakeClient(objs...)
	// OpenShift Image Client Fake
	imgcli := imgfake.NewSimpleClientset(images...).ImageV1()

	// OpenShift Build Client Fake with build for s2i defined, since we'll trigger a build during the reconcile phase
	buildcli := buildfake.NewSimpleClientset().BuildV1()
	moncli := monfake.NewSimpleClientset().MonitoringV1()
	disccli := test.CreateFakeDiscoveryClient()
	// ********** sanity check
	kogitoAppList := &v1alpha1.KogitoAppList{}
	err := cli.List(context.TODO(), kogitoAppList, client.InNamespace("test"))
	if err != nil {
		t.Fatalf("Failed to list kogitoapp (%v)", err)
	}
	assert.True(t, len(kogitoAppList.Items) > 0)
	assert.True(t, kogitoAppList.Items[0].Spec.Resources.Limits[0].Resource == cpuResource)
	cachefake := &cachev1.FakeInformers{}
	// call reconcile object and mock image and build clients
	r := &ReconcileKogitoApp{
		client: &kogitoclient.Client{
			BuildCli:      buildcli,
			ControlCli:    cli,
			ImageCli:      imgcli,
			PrometheusCli: moncli,
			Discovery:     disccli,
		},
		scheme: s,
		cache:  cachefake,
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      kogitoapp.Name,
			Namespace: kogitoapp.Namespace,
		},
	}

	res, err := r.Reconcile(req)
	if err != nil {
		t.Fatalf("reconcile: (%v)", err)
	}
	if !res.Requeue {
		t.Error("reconcile did not requeue request as expected")
	}
	// Let's verify if the objects have been built
	dc := &appsv1.DeploymentConfig{}
	_, err = kubernetes.ResourceC(r.client).FetchWithKey(types.NamespacedName{Name: kogitoapp.Name, Namespace: kogitoapp.Namespace}, dc)
	assert.NoError(t, err)
	assert.NotNil(t, dc)
	assert.Len(t, dc.Spec.Template.Spec.Containers, 1)
	assert.Len(t, dc.GetOwnerReferences(), 1)

	bcS2I := &buildv1.BuildConfig{}
	_, err = kubernetes.ResourceC(r.client).FetchWithKey(types.NamespacedName{Name: kogitoapp.Name + kogitores.BuildS2INameSuffix, Namespace: kogitoapp.Namespace}, bcS2I)
	assert.NoError(t, err)
	assert.NotNil(t, bcS2I)

	assert.Equal(t, resource.MustParse(kogitores.DefaultBuildS2IJVMCPULimit.Value), *bcS2I.Spec.Resources.Limits.Cpu())
	assert.Equal(t, resource.MustParse(kogitores.DefaultBuildS2IJVMMemoryLimit.Value), *bcS2I.Spec.Resources.Limits.Memory())

	for _, isName := range kogitores.ImageStreamNameList {
		hasIs, _ := openshift.ImageStreamC(r.client).FetchTag(types.NamespacedName{Name: isName, Namespace: "test"}, "0.4.0")
		assert.NotNil(t, hasIs)
	}

}

func TestReconcileKogitoApp_updateKogitoAppStatus(t *testing.T) {
	kogitoapp := createFakeKogitoApp()
	kogitoapp.Status = v1alpha1.KogitoAppStatus{
		Builds: v1alpha1.Builds{
			Complete: []string{
				"test-app",
				"test-app-build",
			},
		},
		Route: "http://localhost",
		Conditions: []v1alpha1.Condition{
			{
				Type: v1alpha1.DeployedConditionType,
			},
		},
	}
	images := createFakeImages(kogitoapp.Name)
	s := setupSchema(kogitoapp)
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      kogitoapp.Name,
			Namespace: kogitoapp.Namespace,
		},
	}
	buildconfigRuntime := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kogitoapp.Name,
			Namespace: kogitoapp.Namespace,
			Labels: map[string]string{
				kogitores.LabelKeyAppName:   kogitoapp.Name,
				kogitores.LabelKeyBuildType: string(kogitores.BuildTypeRuntime),
			},
		},
	}
	buildconfigS2I := &buildv1.BuildConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kogitoapp.Name + "-build",
			Namespace: kogitoapp.Namespace,
			Labels: map[string]string{
				kogitores.LabelKeyAppName:   kogitoapp.Name,
				kogitores.LabelKeyBuildType: string(kogitores.BuildTypeS2I),
			},
		},
	}
	buildList := &buildv1.BuildList{
		Items: []buildv1.Build{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kogitoapp.Name,
					Namespace: kogitoapp.Namespace,
					Labels: map[string]string{
						kogitores.LabelKeyAppName:   kogitoapp.Name,
						kogitores.LabelKeyBuildType: string(kogitores.BuildTypeRuntime),
					},
				},
				Status: buildv1.BuildStatus{
					Phase: buildv1.BuildPhaseComplete,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kogitoapp.Name + "-build",
					Namespace: kogitoapp.Namespace,
					Labels: map[string]string{
						kogitores.LabelKeyAppName:   kogitoapp.Name,
						kogitores.LabelKeyBuildType: string(kogitores.BuildTypeS2I),
					},
				},
				Status: buildv1.BuildStatus{
					Phase: buildv1.BuildPhaseComplete,
				},
			},
		},
	}
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kogitoapp.Name,
			Namespace: kogitoapp.Namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "localhost",
		},
	}

	clientfake := &kogitoclient.Client{
		ControlCli:    fake.NewFakeClient(kogitoapp, route),
		BuildCli:      buildfake.NewSimpleClientset(buildconfigRuntime, buildconfigS2I, buildList).BuildV1(),
		ImageCli:      imgfake.NewSimpleClientset(images...).ImageV1(),
		Discovery:     test.CreateFakeDiscoveryClient(),
		PrometheusCli: monfake.NewSimpleClientset().MonitoringV1(),
	}
	kogitoAppResources := &kogitores.KogitoAppResources{
		BuildConfigRuntime: buildconfigRuntime,
		BuildConfigS2I:     buildconfigS2I,
		DeploymentConfig: &appsv1.DeploymentConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kogitoapp.Name,
				Namespace: kogitoapp.Namespace,
			},
		},
		Route: route,
	}

	type fields struct {
		client *kogitoclient.Client
		scheme *runtime.Scheme
		cache  cache.Cache
	}
	type args struct {
		request              *reconcile.Request
		instance             *v1alpha1.KogitoApp
		kogitoResources      *kogitores.KogitoAppResources
		updateResourceResult *status.UpdateResourcesResult
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		result *reconcile.Result
		err    error
	}{
		{
			"Success",
			fields{
				clientfake,
				s,
				mockCache{
					kogitoApp: kogitoapp,
				},
			},
			args{
				&req,
				kogitoapp,
				kogitoAppResources,
				&status.UpdateResourcesResult{},
			},
			&reconcile.Result{},
			nil,
		},
		{
			"Error",
			fields{
				clientfake,
				s,
				&cachev1.FakeInformers{},
			},
			args{
				&req,
				kogitoapp,
				kogitoAppResources,
				&status.UpdateResourcesResult{
					Err: fmt.Errorf("error"),
				},
			},
			&reconcile.Result{},
			fmt.Errorf("error"),
		},
		{
			"Requeue",
			fields{
				clientfake,
				s,
				&cachev1.FakeInformers{},
			},
			args{
				&req,
				kogitoapp,
				kogitoAppResources,
				&status.UpdateResourcesResult{},
			},
			&reconcile.Result{Requeue: true},
			nil,
		},
		{
			"RequeueAfter",
			fields{
				&kogitoclient.Client{
					ControlCli:    fake.NewFakeClient(kogitoapp),
					BuildCli:      buildfake.NewSimpleClientset(buildconfigRuntime, buildconfigS2I, buildList).BuildV1(),
					ImageCli:      imgfake.NewSimpleClientset(images...).ImageV1(),
					Discovery:     test.CreateFakeDiscoveryClient(),
					PrometheusCli: monfake.NewSimpleClientset().MonitoringV1(),
				},
				s,
				mockCache{
					kogitoApp: kogitoapp,
				},
			},
			args{
				&req,
				kogitoapp,
				kogitoAppResources,
				&status.UpdateResourcesResult{},
			},
			&reconcile.Result{RequeueAfter: time.Duration(500) * time.Millisecond},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileKogitoApp{
				client: tt.fields.client,
				scheme: tt.fields.scheme,
				cache:  tt.fields.cache,
			}

			result := &reconcile.Result{}
			var err error

			r.updateKogitoAppStatus(tt.args.request, tt.args.instance, tt.args.kogitoResources, tt.args.updateResourceResult, result, &err)

			if !reflect.DeepEqual(err, tt.err) {
				t.Errorf("updateKogitoAppStatus() error = %v, expectErr %v", err, tt.err)
				return
			}
			if !reflect.DeepEqual(result, tt.result) {
				t.Errorf("updateKogitoAppStatus() result = %v, expectResult %v", result, tt.result)
				return
			}
		})
	}
}

type mockCache struct {
	cache.Cache
	kogitoApp *v1alpha1.KogitoApp
}

func (c mockCache) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	app := obj.(*v1alpha1.KogitoApp)
	app.Spec = c.kogitoApp.Spec
	app.Status = c.kogitoApp.Status
	return nil
}
