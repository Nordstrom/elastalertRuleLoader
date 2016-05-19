# elastalertRuleLoader

Docker container for reading [Yelp elastalert] (https://github.com/Yelp/elastalert) rules from Kubernetes services annotations and saving that in the configured rules folder.

This container needs the following environment variables:
- PLATFORM_INSTANCE_NAME: Name of the platform (used to set index for rules)
