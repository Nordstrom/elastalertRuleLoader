FROM nordstrom/baseimage-ubuntu:16.04
MAINTAINER Innovation Platform Team "invcldtm@nordstrom.com"

COPY ElastalertRuleLoader /bin/ElastalertRuleLoader
RUN chmod 755 /bin/ElastalertRuleLoader


ENTRYPOINT	["/bin/ElastalertRuleLoader"]