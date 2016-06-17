# elastalertRuleLoader

Docker container for reading [Yelp elastalert] (https://github.com/Yelp/elastalert) rules from Kubernetes services annotations and saving that in the configured rules folder.

This container needs the following environment variables:
- PLATFORM_INSTANCE_NAME: Name of the platform (used to set index for rules)
- ELASTICSEARCH_AWS_REGION: Name of aws region where elasticsearch is deployed
- RULES_DIRECTORY: Path where rules (defined in service annotations) will be written
- CONFIG_MAP_DIRECTORY: Path where configmap rules will be read from
