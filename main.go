package main //import "github.com/nordstrom/elastalertRuleLoader"

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	configMapLocation = flag.String("configMapLocation", os.Getenv("CONFIG_MAP_DIRECTORY"), "Location of the config map mount.")
	rulesLocation     = flag.String("rulesDirectory", os.Getenv("RULES_DIRECTORY"), "Path where the rules that come from the services should be written.")
	helpFlag          = flag.Bool("help", false, "")
	annotationKey     = flag.String("annotationKey", "nordstrom.net/elastalertAlerts", "Annotation key for elastalert rules")
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

	if *helpFlag || *rulesLocation == "" || *configMapLocation == "" {
		flag.PrintDefaults()
		os.Exit(0)
	}

	log.Printf("Rule Updater loaded.\n")
	log.Printf("Config Map input path: %s\n", *configMapLocation)
	log.Printf("Rules output path: %s\n", *rulesLocation)

	// create client
	var kubeClient *kclient.Client
	kubeClient, err := kclient.NewInCluster()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// initial configmap rules pull.
	updateConfigMapRules(*configMapLocation, *rulesLocation)

	// initial service rules pull.
	updateServiceRules(kubeClient, *rulesLocation)

	// setup watcher for services
	_ = watchForServices(kubeClient, func(interface{}) {
		log.Printf("Services have updated.\n")
		updateServiceRules(kubeClient, *rulesLocation)
	})

	// setup file watcher, will trigger whenever the configmap updates
	watcher, err := WatchFile(*configMapLocation, time.Second, func() {
		log.Printf("ConfigMap files updated.\n")
		updateConfigMapRules(*configMapLocation, *rulesLocation)
	})
	if err != nil {
		log.Fatalf("Unable to watch ConfigMap: %s\n", err)
	}

	defer func() {
		log.Printf("Cleaning up.")
		watcher.Close()
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

func GatherFilesFromConfigmap(configMapLocation string) []string {
	fileList := []string{}
	err := filepath.Walk(configMapLocation, func(path string, f os.FileInfo, err error) error {
		stat, err := os.Stat(path)
		if err != nil {
			log.Printf("Cannot stat %s, %s\n", path, err)
		}
		if !stat.IsDir() {
			// ignore the configmap /..dirname directories
			if !(strings.Contains(path, "/..")) {
				fileList = append(fileList, path)
			}
		}
		return nil
	})
	if err != nil {
		// not sure what I might see here, so making this fatal for now
		log.Printf("Cannot process path: %s, %s\n", configMapLocation, err)
	}
	return fileList
}

func updateServiceRules(kubeClient *kclient.Client, rulesLocation string) bool {
	log.Println("Processing Service rules.")
	ruleFileExt := ".service.yaml"
	ruleList := gatherRulesFromServices(kubeClient)

	// delete old rules
	cmd := exec.Command("rm", "-rf", fmt.Sprintf("*%s", ruleFileExt))
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
		err = writeRule(erule, rulesLocation, ruleFileExt)
		if err != nil {
			log.Printf("%s\n", err)
		}
	}
	return true
}

func updateConfigMapRules(configMapLocation string, rulesLocation string) {
	log.Println("Processing ConfigMap rules.")
	ruleFileExt := ".configmap.yaml"
	fileList := GatherFilesFromConfigmap(configMapLocation)

	for _, file := range fileList {
		content, err := processRuleFile(file)
		if err != nil {
			log.Println(err)
			continue
		}
		err = writeRule(content, rulesLocation, ruleFileExt)
		if err != nil {
			log.Printf("%s\n", err)
		}
	}
}

func writeRule(rule elastalertRule, rulesLocation, extension string) error {
	filename := fmt.Sprintf("%s/%s%s", rulesLocation, rule.name, extension)
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

func loadConfig(configFile string) string {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Cannot read ConfigMap file: %s\n", err)
	}

	return string(configData)
}

func processRuleFile(file string) (elastalertRule, error) {
	configManager := NewMutexConfigManager(loadConfig(file))
	defer func() {
		configManager.Close()
	}()

	rule := configManager.Get()

	var urule map[string]interface{}
	if err := yaml.Unmarshal([]byte(rule), &urule); err != nil {
		return elastalertRule{}, fmt.Errorf("Unable to unmarshal elastalert rule from configmap supplied file %s. Error: %s; Rule: %s. Skipping rule.\n", file, err, rule)
	}
	eaRule, err := processRule(urule)

	if err != nil {
		return elastalertRule{}, err
	}

	return eaRule, nil
}

func processRule(ruleMap map[string]interface{}) (elastalertRule, error) {
	eaRule := elastalertRule{}
	if str, ok := ruleMap["name"]; ok {
		eaRule.name = str.(string)
	}

	// // Set 'index' if not set
	// if _, ok := ruleMap["index"]; !ok {
	// 	ruleMap["index"] = fmt.Sprintf("%s-*", os.Getenv("PLATFORM_INSTANCE_NAME"))
	// }
	// // Set 'alert' if not set
	// if _, ok := ruleMap["alert"]; !ok {
	// 	ruleMap["alert"] = "elastalert_modules.prometheus_alertmanager.PrometheusAlertManagerAlerter"
	// }
	// // Set 'alertmanager_url' if not set
	// if _, ok := ruleMap["alertmanager_url"]; !ok {
	// 	ruleMap["alertmanager_url"] = fmt.Sprintf("http://%s:%s/", os.Getenv("ALERTMANAGER_SERVICE_HOST"), os.Getenv("ALERTMANAGER_SERVICE_PORT"))
	// }
	// Set 'use_kibana4_dashboard' if not set
	// if _, ok := ruleMap["use_kibana4_dashboard"]; !ok {
	// 	ruleMap["use_kibana4_dashboard"] = "/_plugin/kibana/#/dashboard"
	// }
	// // Set 'use_kibana4_dashboard' if not set
	// if _, ok := ruleMap["aws_region"]; !ok {
	// 	ruleMap["aws_region"] = os.Getenv("ELASTICSEARCH_AWS_REGION")
	// }

	r, err := yaml.Marshal(&ruleMap)
	if err != nil {
		return elastalertRule{}, fmt.Errorf("Unable to marshal elastalert rule. Error: %s; Rule: %s. Skipping rule.", err, ruleMap)
	}

	eaRule.rule = string(r)
	return eaRule, nil
}
