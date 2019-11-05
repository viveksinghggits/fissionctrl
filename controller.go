package main

import (
	"fmt"
	"time"

	fclientset "github.com/fissionctrl/pkg/client/clientset/versioned"
	funcInformers "github.com/fissionctrl/pkg/client/informers/externalversions/fission/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	kubelisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	functionLister "github.com/fissionctrl/pkg/client/listers/fission/v1"
)

type AController struct {
	kubeclientset    kubernetes.Interface
	fissionclientset fclientset.Interface

	deployentLister   kubelisters.DeploymentLister
	deploymentsSynced cache.InformerSynced

	functionLister functionLister.FunctionLister
	functionSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
}

const controllerAgentName = "Sample-Controller"

func NewController(kubeclientset kubernetes.Interface, fissionclientset fclientset.Interface,
	deploymentInformer kinformers.DeploymentInformer, functionInformer funcInformers.FunctionInformer) *AController {
	klog.Info("New Controller has been called")
	controller := &AController{
		kubeclientset:     kubeclientset,
		fissionclientset:  fissionclientset,
		deployentLister:   deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		functionLister:    functionInformer.Lister(),
		functionSynced:    functionInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Functions"),
	}

	klog.Info("Settng up event handlers")
	functionInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.EnqueuFun,
		UpdateFunc: func(old, new interface{}) {
			controller.EnqueuFun(new)
		},
	})

	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

func (c *AController) Run(threadiness int, stopCh <-chan struct{}) error {

	defer c.workqueue.ShutDown()
	klog.Info("Starting function controller")

	// wait for caches to sync up before starting the workers
	klog.Info("Waiting for informers cache to sync up ")
	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced, c.functionSynced); !ok {
		return fmt.Errorf("failed to wait for cache to sync up ")
	}

	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.RunWorker, time.Second, stopCh)
	}

	klog.Info("Started workers ")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *AController) RunWorker() {
	for c.ProcessNextWorkItem() {

	}

}

func (c *AController) ProcessNextWorkItem() bool {

	return true
}

func (c *AController) EnqueuFun(obj interface{}) {
	klog.Info("Enqueue func was called ")
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		klog.Fatalf("Errror while enqueuing the object %s", err.Error())
		return
	}

	c.workqueue.Add(key)

}

func (c *AController) handleObject(obj interface{}) {

}
