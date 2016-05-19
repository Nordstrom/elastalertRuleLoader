package main //import "github.com/nordstrom/elastalertRuleLoader"

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"gopkg.in/yaml.v2"

	kapi "k8s.io/kubernetes/pkg/api"
	kcache "k8s.io/kubernetes/pkg/client/cache"
	kclient "k8s.io/kubernetes/pkg/client/unversioned"
	kframework "k8s.io/kubernetes/pkg/controller/framework"
	kselector "k8s.io/kubernetes/pkg/fields"
	klabels "k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"
)

var (
	// FLAGS
	serviceRulesLocation = flag.String("svrules", os.Getenv("SV_RULES_LOCATION"), "Path where the rules that come from the services should be written.")
	helpFlag             = flag.Bool("help", false, "")
	annotationKey        = flag.String("annotationKey", "nordstrom.net/elastalertAlerts", "Annotation key for elastalert rules")
)

const (
	// Resync period for the kube controller loop.
	resyncPeriod = 30 * time.Minute
	// A subdomain added to the user specified domain for all services.
	serviceSubdomain = "svc"
	// A subdomain added to the user specified dmoain for all pods.
	podSubdomain = "pod"
)

type elastalertRule struct {
	rule string
	name string
}

func main() {
	flag.Parse()

	if *helpFlag || *serviceRulesLocation == "" {
		flag.PrintDefaults()
		os.Exit(0)
	}

	log.Printf("Rule Updater loaded.\n")
	log.Printf("Service Rules location: %s\n", *serviceRulesLocation)

	// create client
	var kubeClient *kclient.Client
	kubeClient, err := kclient.NewInCluster()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// initial service rules pull.
	updateServiceRules(kubeClient, *serviceRulesLocation)

	// setup watcher for services
	_ = watchForServices(kubeClient, func(interface{}) {
		log.Printf("Services have updated.\n")
		updateServiceRules(kubeClient, *serviceRulesLocation)
	})

	defer func() {
		log.Printf("Cleaning up.")
	}()

	select {}
}

func createServiceLW(kubeClient *kclient.Client) *kcache.ListWatch {
	return kcache.NewListWatchFromClient(kubeClient, "services", kapi.NamespaceAll, kselector.Everything())
}

func watchForServices(kubeClient *kclient.Client, callback func(interface{})) kcache.Store {
	serviceStore, serviceController := kframework.NewInformer(
		createServiceLW(kubeClient),
		&kapi.Service{},
		0,
		kframework.ResourceEventHandlerFuncs{
			AddFunc:    callback,
			DeleteFunc: callback,
			UpdateFunc: func(a interface{}, b interface{}) { callback(b) },
		},
	)
	go serviceController.Run(wait.NeverStop)
	return serviceStore
}

func gatherRulesFromServices(kubeClient *kclient.Client) []map[string]interface{} {
	si := kubeClient.Services(kapi.NamespaceAll)
	serviceList, err := si.List(kapi.ListOptions{
		LabelSelector: klabels.Everything(),
		FieldSelector: kselector.Everything()})
	if err != nil {
		log.Printf("Unable to list services: %s", err)
	}

	var ruleList []map[string]interface{}

	for _, svc := range serviceList.Items {
		anno := svc.GetObjectMeta().GetAnnotations()
		name := svc.GetObjectMeta().GetName()
		log.Printf("Processing Service - %s\n", name)

		for k, v := range anno {
			if k == *annotationKey {
				if err := yaml.Unmarshal([]byte(v), &ruleList); err != nil {
					log.Printf("Unable to unmarshal elastalert rule for service %s. Error: %s; Rule: %s. Skipping rule.\n", name, err, v)
				}
			}
		}
	}

	return ruleList
}

func updateServiceRules(kubeClient *kclient.Client, rulesLocation string) bool {
	log.Println("Processing Service rules.")

	ruleList := gatherRulesFromServices(kubeClient)

	// delete old rules
	cmd := exec.Command("rm", "-rf", "*.service.rule")
	log.Printf("Deleting old service rules.\n")
	err := cmd.Start()
	if err != nil {
		log.Printf("Unable to delete old service rules. Error: %s\n", err)
	}
	err = cmd.Wait()
	log.Printf("Command finished with exit code: %v\n", err)

	for _, rule := range ruleList {
		erule, err := processRule(rule)
		if err != nil {
			log.Println(err)
			continue
		}
		err = writeRule(erule, rulesLocation)
		if err != nil {
			log.Printf("%s\n", err)
		}
	}
	return true
}

func writeRule(rule elastalertRule, rulesLocation string) error {
	filename := fmt.Sprintf("%s/%s.service.rule", rulesLocation, rule.name)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("Unable to open rules file %s for writing. Error: %s", filename, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	byteCount, err := w.WriteString(rule.rule)
	if err != nil {
		return fmt.Errorf("Unable to write rule. Rulename: %s Error: %s", rule.name, err)
	}
	log.Printf("Wrote %d bytes.\n", byteCount)
	w.Flush()

	return nil
}

func processRule(ruleMap map[string]interface{}) (elastalertRule, error) {
	eaRule := elastalertRule{}
	if str, ok := ruleMap["name"]; ok {
		eaRule.name = str.(string)
	}

	// Set 'index' if not set
	if _, ok := ruleMap["index"]; !ok {
		ruleMap["index"] = fmt.Sprintf("%s-*", os.Getenv("PLATFORM_INSTANCE_NAME"))
	}
	// Set 'alert' if not set
	if _, ok := ruleMap["alert"]; !ok {
		ruleMap["alert"] = "elastalert_modules.prometheus_alertmanager.PrometheusAlertManagerAlerter"
	}
	// Set 'alertmanager_url' if not set
	if _, ok := ruleMap["alertmanager_url"]; !ok {
		ruleMap["alertmanager_url"] = fmt.Sprintf("http://%s:%s/", os.Getenv("PROMETHEUS_SERVICE_HOST"), os.Getenv("PROMETHEUS_SERVICE_PORT_ALERTMANAGER"))
	}
	// Set 'use_kibana4_dashboard' if not set
	if _, ok := ruleMap["use_kibana4_dashboard"]; !ok {
		ruleMap["use_kibana4_dashboard"] = "/_plugin/kibana/#/dashboard"
	}

	r, err := yaml.Marshal(&ruleMap)
	if err != nil {
		return elastalertRule{}, fmt.Errorf("Unable to marshal elastalert rule. Error: %s; Rule: %s. Skipping rule.", err, ruleMap)
	}

	eaRule.rule = string(r)
	return eaRule, nil
}
