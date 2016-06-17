FROM quay.io/nordstrom/baseimage-ubuntu:16.04
MAINTAINER  Store Modernization Platform Team "invcldtm@nordstrom.com"

COPY elastalertRuleLoader /bin/elastalertRuleLoader

ENTRYPOINT	["/bin/elastalertRuleLoader"]