FROM nordstrom/baseimage-ubuntu:16.04
MAINTAINER  Store Modernization Platform Team "invcldtm@nordstrom.com"

COPY elastalertRuleLoader /bin/elastalertRuleLoader
RUN chmod 755 /bin/elastalertRuleLoader

ENTRYPOINT	["/bin/elastalertRuleLoader"]