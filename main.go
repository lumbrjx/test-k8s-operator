package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func createDaemonSet(clientset *kubernetes.Clientset) error {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "grpc-server-daemonset",
			Labels: map[string]string{
				"app": "grpc-server",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "grpc-server",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "grpc-server",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							// Name:  "grpc-server",
							// Image: "your-docker-repo/grpc-server:latest",
							// Ports: []corev1.ContainerPort{
							// 	{
							// 		ContainerPort: 50051,
							// 	},
							// },
							Name:  "busybox",
							Image: "busybox",
							Command: []string{
								"sh",
								"-c",
								"while true; do echo Hello from the BusyBox DaemonSet; sleep 3600; done",
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().
		DaemonSets("default").
		Create(context.TODO(), ds, metav1.CreateOptions{})
	return err
}

func main() {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Println("Falling back to in-cluster config")
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	hellowrld := schema.GroupVersionResource{
		Group:    "hlw.io",
		Version:  "v1",
		Resource: "hellowrlds",
	}

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dynClient.Resource(hellowrld).
					Namespace("").
					List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dynClient.Resource(hellowrld).
					Namespace("").
					Watch(context.TODO(), options)
			},
		},
		&unstructured.Unstructured{},
		0, // Skip resync
		cache.Indexers{},
	)

	// Event handlers
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			log.Println("Add event detected:", obj)
			err := createDaemonSet(clientset)
			if err != nil {
				log.Fatalf("Failed to create DaemonSet: %v", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			log.Println("Update event detected:", newObj)
		},
		DeleteFunc: func(obj interface{}) {
			log.Println("Delete event detected:", obj)
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)

	go informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
		log.Fatalf("Timeout waiting for informer sync")
	}

	// Run until channel is closed
	log.Println("Custom Resource Controller started successfully")
	<-stopCh
}
