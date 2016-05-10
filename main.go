package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
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

func gatherRulesFromServices(kubeClient *kclient.Client) []string {
	si := kubeClient.Services(kapi.NamespaceAll)
	serviceList, err := si.List(kapi.ListOptions{
		LabelSelector: klabels.Everything(),
		FieldSelector: kselector.Everything()})
	if err != nil {
		log.Printf("Unable to list services: %s", err)
	}

	ruleList := []string{}

	for _, svc := range serviceList.Items {
		anno := svc.GetObjectMeta().GetAnnotations()
		name := svc.GetObjectMeta().GetName()
		log.Printf("Processing Service - %s\n", name)

		for k, v := range anno {
			log.Printf("- %s", k)
			if k == "nordstrom.net/elastalertAlerts" {
				var alerts interface{}
				err := json.Unmarshal([]byte(v), &alerts)
				if err != nil {
					log.Printf("Error decoding json object that contains alert(s): %s\n", err)
				}
				if reflect.TypeOf(alerts).Kind() == reflect.Slice {
					collection := reflect.ValueOf(alerts)
					for i := 0; i < collection.Len(); i++ {
						s := collection.Index(i).Interface().(string)
						ruleList = append(ruleList, s)
					}
				}
				if reflect.TypeOf(alerts).Kind() == reflect.String {
					ruleList = append(ruleList, reflect.ValueOf(alerts).String())
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

func processRule(inRule string) (elastalertRule, error) {
	m := make(map[interface{}]interface{})

	err := yaml.Unmarshal([]byte(inRule), &m)
	if err != nil {
		return elastalertRule{}, fmt.Errorf("Unable to unmarshal rule with error: %s rule: %s, skipping.", err, inRule)
	}

	earule := elastalertRule{}
	if str, ok := m["name"].(string); ok {
		earule.name = str
	}
	earule.rule = inRule

	return earule, nil
}
