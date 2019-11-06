package main

import (
	"fmt"
	"time"

	fclientset "github.com/fissionctrl/pkg/client/clientset/versioned"
	funcInformers "github.com/fissionctrl/pkg/client/informers/externalversions/fission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	kubelisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	functionv1 "github.com/fissionctrl/pkg/apis/fission/v1"
	functionscheme "github.com/fissionctrl/pkg/client/clientset/versioned/scheme"
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
	recorder  record.EventRecorder
}

const controllerAgentName = "Sample-Controller"

func NewController(kubeclientset kubernetes.Interface, fissionclientset fclientset.Interface,
	deploymentInformer kinformers.DeploymentInformer, functionInformer funcInformers.FunctionInformer) *AController {
	klog.Info("New Controller has been called")

	utilruntime.Must(functionscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadCaster := record.NewBroadcaster()
	eventBroadCaster.StartLogging(klog.Infof)
	eventBroadCaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})

	controller := &AController{
		kubeclientset:     kubeclientset,
		fissionclientset:  fissionclientset,
		deployentLister:   deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		functionLister:    functionInformer.Lister(),
		functionSynced:    functionInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Functions"),
		recorder:          eventBroadCaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName}),
	}

	klog.Info("Settng up event handlers")
	functionInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.EnqueuFun,
		UpdateFunc: func(old, new interface{}) {
			controller.EnqueuFun(new)
		},
		DeleteFunc: controller.EnqueuFun,
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
	// get the object to work on
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	err := func(obj interface{}) error {
		/*
			we are calling Done here to let the workqueue know that we
			are done processing the obj here
		*/
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// the object that we get from the the queue is string which will be in the format
		// namespace/resourcename. ...
		if key, ok = obj.(string); !ok {
			// the object that was in queue is not string (because we were not able to convert that)
			// so we can forget that object
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("Expecting string in workququq but got %#v", obj))
			return nil
		}

		// run the synchandler passing it the object that we have in the queue in the form namespace/resname
		// if there is an error in the synchandler we will add the object in the queue again or requeue that object
		if err := c.SyncHandler(key); err != nil {
			// there was an error handling this key put it back in the queue
			klog.Fatalf("Error in synchhanderl returned this %s", err.Error())
			c.workqueue.AddRateLimited(key)
			klog.Fatalf("There was an error syncing the object %s, error: %s", key, err.Error())
			return fmt.Errorf("error sysncing %s: %s requeueing ", key, err.Error())
		}

		// if there was no error the object was synced successfully, we can forget that object now

		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced %s", key)
		return nil
	}(obj)
	if err != nil {

		utilruntime.HandleError(err)
		return true
	}
	return true
}

func (c *AController) DeleteDeployment(name string, namespace string) error {
	klog.Infof("Deleting the deployment %s", name)
	err := c.kubeclientset.AppsV1().Deployments(namespace).Delete(name, &metav1.DeleteOptions{})
	return err
}

func (c *AController) SyncHandler(key string) error {
	// convert the namespace and name into distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Fatalf("There was an error splitting the namespace/name: ", err.Error())
		return nil
	}
	// get the function resource with this namespace/name
	// remember we are storing the entire object in the queue, we are just storing the key i.e. namespace/name
	function, err := c.functionLister.Functions(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			// function resource is not found
			// delete the deployment that was created
			// by this function
			err := c.DeleteDeployment(name, namespace)
			if err != nil {
				klog.Fatalf("There was an error deleting the deployment %s", err.Error())
			}

			return nil
		}
	}

	deploymentName := function.Name
	if deploymentName == "" {
		return nil
	}

	// get the deployment from the name that we have and the namespace
	deployment, err := c.deployentLister.Deployments(function.Namespace).Get(deploymentName)

	// now if the deployment doesnt exist we will create it...
	if errors.IsNotFound(err) {
		klog.Info("Creating the deployment %s", deploymentName)
		deployment, err = c.kubeclientset.AppsV1().Deployments(function.Namespace).Create(newDeployment(function))

	}

	// if there was an error creating/getting the deployment we will requeue that object
	if err != nil {
		klog.Fatalf("There was an error creating/getting the deployment %s ", err.Error())
		return err
	}

	// finally update the Function status
	// after creating the deployment if we have something to update in the function resourcec we can do that now
	// for example if we are maintaining the replica count we update the replicas

	klog.Info("The deployment %s was created successfully.", deployment.Name)
	return nil
}

func newDeployment(function *functionv1.Function) *appsv1.Deployment {
	labels := map[string]string{
		"app":        function.Name,
		"controller": function.Name,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      function.Name,
			Namespace: function.Namespace,
			// OwnerReferences: []metav1.OwnerReference{
			// 	*metav1.NewControllerRef(function, functionv1.SchemeGroupVersion.WithKind("Function")),
			// },
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &function.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "continername",
							Image: function.Spec.Image.Name,
						},
					},
				},
			},
		},
	}
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
