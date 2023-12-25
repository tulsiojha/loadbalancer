/*
Copyright 2023.

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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/tulsiojha/operators/api/v1alpha1"
)

// LoadbalancerReconciler reconciles a Loadbalancer object
type LoadbalancerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cache.kloudlite.io,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.kloudlite.io,resources=loadbalancers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.kloudlite.io,resources=loadbalancers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Loadbalancer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *LoadbalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	v := &cachev1alpha1.Loadbalancer{}
	err := r.Get(ctx, req.NamespacedName, v)

	if err != nil {
		log.Log.Info("resource not found")
		return ctrl.Result{}, err
	}

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		return ctrl.Result{}, err
	}

	var internalIP string = ""

	for _, node := range nodeList.Items {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP {
				internalIP = address.Address
				break
			}
		}
	}

	log.Log.Info("Internal ip " + internalIP)
	serviceList := &corev1.ServiceList{}
	log.Log.Info("no no..")
	if err = r.List(ctx, serviceList); err != nil {
		return ctrl.Result{}, err
	}

	exposedNodePorts := []corev1.ServicePort{}

	for _, svc1 := range serviceList.Items {

		if svc1.Spec.Type == corev1.ServiceTypeNodePort {
			ports := svc1.Spec.Ports
			for _, port := range ports {
				log.Log.Info(fmt.Sprintf("%d", port.NodePort))
				exposedNodePorts = append(exposedNodePorts, corev1.ServicePort{
					Port:       port.NodePort,
					TargetPort: intstr.FromInt(int(port.NodePort)),
					Protocol:   corev1.ProtocolTCP,
					Name:       "serviceport-" + fmt.Sprintf("%d", port.NodePort),
				})
			}
		}
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, existingSvc)
	if err != nil {
		svc := r.createService(v, &exposedNodePorts)
		error := r.Create(ctx, svc)
		if error != nil {
			log.Log.Error(error, "Error creating service")
			return ctrl.Result{}, error
		}
	}

	existingDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, existingDeploy)

	if err != nil {
		deploy := r.createDeployment(v)
		error := r.Create(ctx, deploy)

		if error != nil {
			log.Log.Error(error, "Error creating deployment")
			return ctrl.Result{}, error
		}
	}

	return ctrl.Result{}, nil
}

func (r *LoadbalancerReconciler) createService(loadBalancer *cachev1alpha1.Loadbalancer, ports *[]corev1.ServicePort) *corev1.Service {
	log.Log.Info("namespace " + loadBalancer.Namespace)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "loadbalancer-service",
			Namespace: "bikash",
		},
		Spec: corev1.ServiceSpec{
			Ports: *ports,
			Selector: map[string]string{
				"app": "loadbalancer-pod",
			},
		},
	}

	return svc
}

func (r *LoadbalancerReconciler) createDeployment(loadbalancer *cachev1alpha1.Loadbalancer) *appsv1.Deployment {
	var replica int32 = 1
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "loadbalancer-deployment",
			Namespace: "bikash",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replica,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "loadbalancer-pod",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "loadbalancer-pod",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "go",
						Image: "ghcr.io/tulsiojha/sample:v1",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Protocol:      corev1.ProtocolTCP,
						}},
						ImagePullPolicy: corev1.PullAlways,
					}},
				},
			},
		},
	}

	return deployment
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadbalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Loadbalancer{}).
		Complete(r)
}
