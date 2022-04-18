/*
Copyright 2022.

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

package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Ingress controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		IngressName      = "test-ingress"
		IngressNamespace = "default"
		ServiceName      = "test-service"
		PathValue        = "/TestPath"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating an ingress", func() {
		It("Should create an ingress", func() {
			By("By creating a new ingress")
			ctx := context.Background()

			backend := networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: ServiceName,
					Port: networkingv1.ServiceBackendPort{
						Number: 100,
					},
				},
			}

			pathType := networkingv1.PathTypeImplementationSpecific
			rule := networkingv1.IngressRule{}
			rule.HTTP = &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						PathType: &pathType,
						Path:     PathValue,
						Backend:  backend,
					},
				},
			}

			ingress := &networkingv1.Ingress{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "networking.k8s.io/v1",
					Kind:       "Ingress",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      IngressName,
					Namespace: IngressNamespace,
				},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{rule},
				},
			}
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())
		})
	})
})
