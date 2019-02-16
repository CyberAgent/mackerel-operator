package Mackerel

import (
	"context"
	"reflect"

	apiv1alpha1 "github.com/mackerel-operator/pkg/apis/kirishikistudios/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_Mackerel")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Mackerel Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMackerel{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("Mackerel-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Mackerel
	err = c.Watch(&source.Kind{Type: &apiv1alpha1.Mackerel{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Mackerel
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apiv1alpha1.Mackerel{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMackerel{}

// ReconcileMackerel reconciles a Mackerel object
type ReconcileMackerel struct {
	// TODO: Clarify the split client
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Mackerel object and makes changes based on the state read
// and what is in the Mackerel.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Mackerel Deployment for each Mackerel CR
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMackerel) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Mackerel")

	// Fetch the Mackerel instance
	Mackerel := &apiv1alpha1.Mackerel{}
	err := r.client.Get(context.TODO(), request.NamespacedName, Mackerel)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Mackerel resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Mackerel")
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.DaemonSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: Mackerel.Name, Namespace: Mackerel.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new Daemonset
		dep := r.daemonsetForMackerel(Mackerel)
		reqLogger.Info("Creating a new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Daemonset", "Daemonset.Namespace", dep.Namespace, "Daemonset.Name", dep.Name)
			return reconcile.Result{}, err
		}
		// Daemonset created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Daemonset")
		return reconcile.Result{}, err
	}

	// Update the Mackerel status with the pod names
	// List the pods for this Mackerel's Daemonset
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(labelsForMackerel(Mackerel.Name))
	listOps := &client.ListOptions{Namespace: Mackerel.Namespace, LabelSelector: labelSelector}
	err = r.client.List(context.TODO(), listOps, podList)
	if err != nil {
		reqLogger.Error(err, "Failed to list pods", "Mackerel.Namespace", Mackerel.Namespace, "Mackerel.Name", Mackerel.Name)
		return reconcile.Result{}, err
	}
	podNames := getPodNames(podList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, Mackerel.Status.Nodes) {
		Mackerel.Status.Nodes = podNames
		err := r.client.Status().Update(context.TODO(), Mackerel)
		if err != nil {
			reqLogger.Error(err, "Failed to update Mackerel status")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// daemonsetForMackerel returns a Mackerel Daemonset object
func (r *ReconcileMackerel) daemonsetForMackerel(m *apiv1alpha1.Mackerel) *appsv1.DaemonSet {
	ls := labelsForMackerel(m.Name)

	dep := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "extensions/v1beta1",
			Kind:       "DaemonSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "mackerel/mackerel-agent:latest",
						Name:  "mackerel-agent",
						Env: []corev1.EnvVar{
							{
								Name:  "apikey",
								//TODO get from ConfigMap or Secret
								Value: "xxxxxxxxxxxxxx",
							},
							{
								//TODO get from ConfigMap or Secret
								Name:  "opts",
								Value: "-role=minikube:mbp13",
							},
							{
								Name:  "enable_docker_plugin",
								Value: "1",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name: "docker-sock",
								MountPath: "/var/run/docker.sock",
							},
							{
								Name: "mackerel-id",
								MountPath: "/var/lib/mackerel-agent/",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "docker-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
							},
						},
						{
							Name: "mackerel-id",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/mackerel-agent/"},
							},
						},
					},
				},
			},
		},
	}
	// Set Mackerel instance as the owner and controller
	_ = controllerutil.SetControllerReference(m, dep, r.scheme)
	return dep
}

// labelsForMackerel returns the labels for selecting the resources
// belonging to the given Mackerel CR name.
func labelsForMackerel(name string) map[string]string {
	return map[string]string{"app": "mackerel-agent", "Mackerel_cr": name}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}
