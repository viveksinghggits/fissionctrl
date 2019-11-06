package main

import (
	"flag"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	fclientset "github.com/fissionctrl/pkg/client/clientset/versioned"
	fissioninformer "github.com/fissionctrl/pkg/client/informers/externalversions"
	"github.com/fissionctrl/pkg/signals"
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	klog.Info("Main Called")
	klog.InitFlags(nil)
	flag.Parse()

	stopCh := signals.SetupSignalHandler()

	// BuildConfigFromFlags builds config from master's URL or from kubeconfig file path.
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("There was an error getting config from flags %s", err.Error())
	}

	// NewForConfig returns a new clienset for the given config
	kubeclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("There was an error getting kubeclient %s", err.Error())
	}

	fissionclient, err := fclientset.NewForConfig(cfg)

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclient, time.Second*30)
	fissionInformerFactory := fissioninformer.NewSharedInformerFactory(fissionclient, time.Second*30)

	controller := NewController(kubeclient, fissionclient, kubeInformerFactory.Apps().V1().Deployments(), fissionInformerFactory.Fission().V1().Functions())

	kubeInformerFactory.Start(stopCh)
	fissionInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error while running controlelr %s", err.Error())
	}

	klog.Info(controller)

}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "location of your kubeconfig file")
	flag.StringVar(&masterURL, "masterURL", "", "URL to your kubeapiserver ")
}
